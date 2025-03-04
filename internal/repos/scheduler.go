package repos

import (
	"container/heap"
	"context"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/grafana/regexp"
	"github.com/inconshreveable/log15"

	"github.com/sourcegraph/sourcegraph/internal/api"
	"github.com/sourcegraph/sourcegraph/internal/conf"
	"github.com/sourcegraph/sourcegraph/internal/database"
	"github.com/sourcegraph/sourcegraph/internal/database/dbutil"
	"github.com/sourcegraph/sourcegraph/internal/gitserver"
	gitserverprotocol "github.com/sourcegraph/sourcegraph/internal/gitserver/protocol"
	"github.com/sourcegraph/sourcegraph/internal/mutablelimiter"
	"github.com/sourcegraph/sourcegraph/internal/repoupdater/protocol"
	"github.com/sourcegraph/sourcegraph/internal/types"
)

// schedulerConfig tracks the active scheduler configuration.
type schedulerConfig struct {
	running               bool
	autoGitUpdatesEnabled bool
}

// RunScheduler runs the worker that schedules git fetches of synced repositories in git-server.
func RunScheduler(ctx context.Context, scheduler *updateScheduler) {
	var (
		have schedulerConfig
		stop context.CancelFunc
	)

	conf.Watch(func() {
		c := conf.Get()

		want := schedulerConfig{
			running:               true,
			autoGitUpdatesEnabled: !c.DisableAutoGitUpdates,
		}

		if have == want {
			return
		}

		if stop != nil {
			stop()
			log15.Info("stopped previous scheduler")
		}

		// We setup a separate sub-context so that we can reuse the original
		// parent context every time we're starting up the newly configured
		// scheduler. If we'd assign to ctx it'd only be usable up until the
		// we'd call stop.
		var ctx2 context.Context
		ctx2, stop = context.WithCancel(ctx)

		go scheduler.runUpdateLoop(ctx2)
		if want.autoGitUpdatesEnabled {
			go scheduler.runScheduleLoop(ctx2)
		}

		log15.Debug(
			"started configured scheduler",
			"version", "new",
			"auto-git-updates", want.autoGitUpdatesEnabled,
		)

		// We converged to the desired configuration.
		have = want

		// Assigning stop to _ makes go-lint not report a false positive context leak.
		_ = stop
	})
}

const (
	// minDelay is the minimum amount of time between scheduled updates for a single repository.
	minDelay = 45 * time.Second

	// maxDelay is the maximum amount of time between scheduled updates for a single repository.
	maxDelay = 8 * time.Hour
)

// updateScheduler schedules repo update (or clone) requests to gitserver.
//
// Repository metadata is synced from configured code hosts and added to the scheduler.
//
// Updates are scheduled based on the time that has elapsed since the last commit
// divided by a constant factor of 2. For example, if a repo's last commit was 8 hours ago
// then the next update will be scheduled 4 hours from now. If there are still no new commits,
// then the next update will be scheduled 6 hours from then.
// This heuristic is simple to compute and has nice backoff properties.
//
// If an error occurs when attempting to fetch a repo we perform exponential
// backoff by doubling the current interval. This ensures that problematic repos
// don't stay in the front of the schedule clogging up the queue.
//
// When it is time for a repo to update, the scheduler inserts the repo into a queue.
//
// A worker continuously dequeues repos and sends updates to gitserver, but its concurrency
// is limited by the gitMaxConcurrentClones site configuration.
type updateScheduler struct {
	updateQueue *updateQueue
	schedule    *schedule
}

// A configuredRepo represents the configuration data for a given repo from
// a configuration source, such as information retrieved from GitHub for a
// given GitHubConnection.
type configuredRepo struct {
	ID   api.RepoID
	Name api.RepoName
}

// notifyChanBuffer controls the buffer size of notification channels.
// It is important that this value is 1 so that we can perform lossless
// non-blocking sends.
const notifyChanBuffer = 1

// NewUpdateScheduler returns a new scheduler.
func NewUpdateScheduler() *updateScheduler {
	return &updateScheduler{
		updateQueue: &updateQueue{
			index:         make(map[api.RepoID]*repoUpdate),
			notifyEnqueue: make(chan struct{}, notifyChanBuffer),
		},
		schedule: &schedule{
			index:         make(map[api.RepoID]*scheduledRepoUpdate),
			wakeup:        make(chan struct{}, notifyChanBuffer),
			randGenerator: rand.New(rand.NewSource(time.Now().UnixNano())),
		},
	}
}

// runScheduleLoop starts the loop that schedules updates by enqueuing them into the updateQueue.
func (s *updateScheduler) runScheduleLoop(ctx context.Context) {
	for {
		select {
		case <-s.schedule.wakeup:
		case <-ctx.Done():
			s.schedule.reset()
			return
		}

		s.runSchedule()
		schedLoops.Inc()
	}
}

func (s *updateScheduler) runSchedule() {
	s.schedule.mu.Lock()
	defer s.schedule.mu.Unlock()
	defer s.schedule.rescheduleTimer()

	for len(s.schedule.heap) != 0 {
		repoUpdate := s.schedule.heap[0]
		if !repoUpdate.Due.Before(timeNow().Add(time.Millisecond)) {
			break
		}

		schedAutoFetch.Inc()
		s.updateQueue.enqueue(repoUpdate.Repo, priorityLow)
		repoUpdate.Due = timeNow().Add(repoUpdate.Interval)
		heap.Fix(s.schedule, 0)
	}
}

// runUpdateLoop sends repo update requests to gitserver.
func (s *updateScheduler) runUpdateLoop(ctx context.Context) {
	limiter := configuredLimiter()

	for {
		select {
		case <-s.updateQueue.notifyEnqueue:
		case <-ctx.Done():
			s.updateQueue.reset()
			return
		}

		for {
			ctx, cancel, err := limiter.Acquire(ctx)
			if err != nil {
				// context is canceled; shutdown
				return
			}

			repo, ok := s.updateQueue.acquireNext()
			if !ok {
				cancel()
				break
			}

			go func(ctx context.Context, repo configuredRepo, cancel context.CancelFunc) {
				defer cancel()
				defer s.updateQueue.remove(repo, true)

				// This is a blocking call since the repo will be cloned synchronously by gitserver
				// if it doesn't exist or update it if it does. The timeout of this request depends
				// on the value of conf.GitLongCommandTimeout() or if the passed context has a set
				// deadline shorter than the value of this config.
				resp, err := requestRepoUpdate(ctx, repo, 1*time.Second)
				if err != nil {
					schedError.WithLabelValues("requestRepoUpdate").Inc()
					log15.Error("runUpdateLoop: error requesting repo update", "uri", repo.Name, "err", err)
				} else if resp != nil && resp.Error != "" {
					schedError.WithLabelValues("repoUpdateResponse").Inc()
					log15.Error("runUpdateLoop: error updating repo", "uri", repo.Name, "err", resp.Error)
				}

				if interval := getCustomInterval(conf.Get(), string(repo.Name)); interval > 0 {
					s.schedule.updateInterval(repo, interval)
					return
				}

				if err != nil || (resp != nil && resp.Error != "") {
					// On error we will double the current interval so that we back off and don't
					// get stuck with problematic repos with low intervals.
					if currentInterval, ok := s.schedule.getCurrentInterval(repo); ok {
						s.schedule.updateInterval(repo, currentInterval*2)
					}
				} else if resp != nil && resp.LastFetched != nil && resp.LastChanged != nil {
					// This is the heuristic that is described in the updateScheduler documentation.
					// Update that documentation if you update this logic.
					interval := resp.LastFetched.Sub(*resp.LastChanged) / 2
					s.schedule.updateInterval(repo, interval)
				}
			}(ctx, repo, cancel)
		}
	}
}

func getCustomInterval(c *conf.Unified, repoName string) time.Duration {
	if c == nil {
		return 0
	}
	for _, rule := range c.GitUpdateInterval {
		re, err := regexp.Compile(rule.Pattern)
		if err != nil {
			log15.Warn("error compiling GitUpdateInterval pattern", "error", err)
			continue
		}
		if re.MatchString(repoName) {
			return time.Duration(rule.Interval) * time.Minute
		}
	}
	return 0
}

// requestRepoUpdate sends a request to gitserver to request an update.
var requestRepoUpdate = func(ctx context.Context, repo configuredRepo, since time.Duration) (*gitserverprotocol.RepoUpdateResponse, error) {
	return gitserver.DefaultClient.RequestRepoUpdate(ctx, repo.Name, since)
}

// configuredLimiter returns a mutable limiter that is
// configured with the maximum number of concurrent update
// requests that repo-updater should send to gitserver.
var configuredLimiter = func() *mutablelimiter.Limiter {
	limiter := mutablelimiter.New(1)
	conf.Watch(func() {
		limiter.SetLimit(conf.GitMaxConcurrentClones())
	})
	return limiter
}

// UpdateFromDiff updates the scheduled and queued repos from the given sync
// diff.
//
// We upsert all repos that exist to the scheduler. This is so the
// scheduler can track the repositories and periodically update
// them.
//
// Items on the update queue will be cloned/fetched as soon as
// possible. We treat repos differently depending on which part of the
// diff they are:
//
//
//   Deleted    - remove from scheduler and queue.
//   Added      - new repo, enqueue for asap clone.
//   Modified   - likely new url or name. May also be a sign of new
//                commits. Enqueue for asap clone (or fetch).
//   Unmodified - we likely already have this cloned. Just rely on
//                the scheduler and do not enqueue.
func (s *updateScheduler) UpdateFromDiff(diff Diff) {
	for _, r := range diff.Deleted {
		s.remove(r)
	}

	for _, r := range diff.Added {
		s.upsert(r, true)
	}
	for _, r := range diff.Modified {
		s.upsert(r, true)
	}

	known := len(diff.Added) + len(diff.Modified)
	for _, r := range diff.Unmodified {
		if r.IsDeleted() {
			s.remove(r)
			continue
		}

		known++
		s.upsert(r, false)
	}
}

// PrioritiseUncloned will treat any repos listed in names as uncloned, which in effect
// will move them to the front of he queue for updating ASAP.
//
// This method should be called periodically with the list of all repositories
// managed by the scheduler that are not cloned on gitserver.
func (s *updateScheduler) PrioritiseUncloned(names []string) {
	s.schedule.prioritiseUncloned(names)
}

// EnsureScheduled ensures that all repos in repos exist in the scheduler.
func (s *updateScheduler) EnsureScheduled(repos []types.MinimalRepo) {
	s.schedule.insertNew(repos)
}

// ListRepos list all repos managed by the scheduler
func (s *updateScheduler) ListRepos() []string {
	s.schedule.mu.Lock()
	defer s.schedule.mu.Unlock()

	names := make([]string, len(s.schedule.heap))
	for i := range s.schedule.heap {
		names[i] = string(s.schedule.heap[i].Repo.Name)
	}
	return names
}

// upsert adds r to the scheduler for periodic updates. If r.ID is already in
// the scheduler, then the fields are updated (upsert).
//
// If enqueue is true then r is also enqueued to the update queue for a git
// fetch/clone soon.
func (s *updateScheduler) upsert(r *types.Repo, enqueue bool) {
	repo := configuredRepoFromRepo(r)

	updated := s.schedule.upsert(repo)
	log15.Debug("scheduler.schedule.upserted", "repo", r.Name, "updated", updated)

	if !enqueue {
		return
	}
	updated = s.updateQueue.enqueue(repo, priorityLow)
	log15.Debug("scheduler.updateQueue.enqueued", "repo", r.Name, "updated", updated)
}

func (s *updateScheduler) remove(r *types.Repo) {
	repo := configuredRepoFromRepo(r)

	if s.schedule.remove(repo) {
		log15.Debug("scheduler.schedule.removed", "repo", r.Name)
	}

	if s.updateQueue.remove(repo, false) {
		log15.Debug("scheduler.updateQueue.removed", "repo", r.Name)
	}
}

func configuredRepoFromRepo(r *types.Repo) configuredRepo {
	repo := configuredRepo{
		ID:   r.ID,
		Name: r.Name,
	}

	return repo
}

// UpdateOnce causes a single update of the given repository.
// It neither adds nor removes the repo from the schedule.
func (s *updateScheduler) UpdateOnce(id api.RepoID, name api.RepoName) {
	repo := configuredRepo{
		ID:   id,
		Name: name,
	}
	schedManualFetch.Inc()
	s.updateQueue.enqueue(repo, priorityHigh)
}

// DebugDump returns the state of the update scheduler for debugging.
func (s *updateScheduler) DebugDump(ctx context.Context, db dbutil.DB) interface{} {
	data := struct {
		Name        string
		UpdateQueue []*repoUpdate
		Schedule    []*scheduledRepoUpdate
		SyncJobs    []*types.ExternalServiceSyncJob
	}{
		Name: "repos",
	}

	s.schedule.mu.Lock()
	schedule := schedule{
		heap: make([]*scheduledRepoUpdate, len(s.schedule.heap)),
	}
	for i, update := range s.schedule.heap {
		// Copy the scheduledRepoUpdate as a value so that
		// popping off the heap here won't update the index value of the real heap, and
		// we don't do a racy read on the repo pointer which may change concurrently in the real heap.
		updateCopy := *update
		schedule.heap[i] = &updateCopy
	}
	s.schedule.mu.Unlock()

	for len(schedule.heap) > 0 {
		update := heap.Pop(&schedule).(*scheduledRepoUpdate)
		data.Schedule = append(data.Schedule, update)
	}

	s.updateQueue.mu.Lock()
	updateQueue := updateQueue{
		heap: make([]*repoUpdate, len(s.updateQueue.heap)),
	}
	for i, update := range s.updateQueue.heap {
		// Copy the repoUpdate as a value so that
		// popping off the heap here won't update the index value of the real heap, and
		// we don't do a racy read on the repo pointer which may change concurrently in the real heap.
		updateCopy := *update
		updateQueue.heap[i] = &updateCopy
	}
	s.updateQueue.mu.Unlock()

	for len(updateQueue.heap) > 0 {
		// Copy the scheduledRepoUpdate as a value so that the repo pointer
		// won't change concurrently after we release the lock.
		update := heap.Pop(&updateQueue).(*repoUpdate)
		data.UpdateQueue = append(data.UpdateQueue, update)
	}

	var err error
	data.SyncJobs, err = database.ExternalServices(db).GetSyncJobs(ctx)
	if err != nil {
		log15.Warn("Getting external service sync jobs foe debug page", "error", err)
	}

	return &data
}

// ScheduleInfo returns the current schedule info for a repo.
func (s *updateScheduler) ScheduleInfo(id api.RepoID) *protocol.RepoUpdateSchedulerInfoResult {
	var result protocol.RepoUpdateSchedulerInfoResult

	s.schedule.mu.Lock()
	if update := s.schedule.index[id]; update != nil {
		result.Schedule = &protocol.RepoScheduleState{
			Index:           update.Index,
			Total:           len(s.schedule.index),
			IntervalSeconds: int(update.Interval / time.Second),
			Due:             update.Due,
		}
	}
	s.schedule.mu.Unlock()

	s.updateQueue.mu.Lock()
	if update := s.updateQueue.index[id]; update != nil {
		result.Queue = &protocol.RepoQueueState{
			Index:    update.Index,
			Total:    len(s.updateQueue.index),
			Updating: update.Updating,
		}
	}
	s.updateQueue.mu.Unlock()

	return &result
}

// updateQueue is a priority queue of repos to update.
// A repo can't have more than one location in the queue.
// Implements heap.Interface and sort.Interface.
type updateQueue struct {
	mu sync.Mutex

	heap  []*repoUpdate
	index map[api.RepoID]*repoUpdate

	seq uint64

	// The queue performs a non-blocking send on this channel
	// when a new value is enqueued so that the update loop
	// can wake up if it is idle.
	notifyEnqueue chan struct{}
}

type priority int

const (
	priorityLow priority = iota
	priorityHigh
)

// repoUpdate is a repository that has been queued for an update.
type repoUpdate struct {
	Repo     configuredRepo
	Priority priority
	Seq      uint64 // the sequence number of the update
	Updating bool   // whether the repo has been acquired for update
	Index    int    `json:"-"` // the index in the heap
}

func (q *updateQueue) reset() {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.heap = q.heap[:0]
	q.index = map[api.RepoID]*repoUpdate{}
	q.seq = 0
	q.notifyEnqueue = make(chan struct{}, notifyChanBuffer)

	schedUpdateQueueLength.Set(0)
}

// enqueue adds the repo to the queue with the given priority.
//
// If the repo is already in the queue and it isn't yet updating,
// the repo is updated.
//
// If the given priority is higher than the one in the queue,
// the repo's position in the queue is updated accordingly.
func (q *updateQueue) enqueue(repo configuredRepo, p priority) (updated bool) {
	if repo.ID == 0 {
		panic("repo.id is zero")
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	update := q.index[repo.ID]
	if update == nil {
		heap.Push(q, &repoUpdate{
			Repo:     repo,
			Priority: p,
		})
		notify(q.notifyEnqueue)
		return false
	}

	if update.Updating {
		return false
	}

	update.Repo = repo
	if p <= update.Priority {
		// Repo is already in the queue with at least as good priority.
		return true
	}

	// Repo is in the queue at a lower priority.
	update.Priority = p      // bump the priority
	update.Seq = q.nextSeq() // put it after all existing updates with this priority
	heap.Fix(q, update.Index)
	notify(q.notifyEnqueue)

	return true
}

// nextSeq increments and returns the next sequence number.
// The caller must hold the lock on q.mu.
func (q *updateQueue) nextSeq() uint64 {
	q.seq++
	return q.seq
}

// remove removes the repo from the queue if the repo.Updating matches the updating argument.
func (q *updateQueue) remove(repo configuredRepo, updating bool) (removed bool) {
	if repo.ID == 0 {
		panic("repo.id is zero")
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	update := q.index[repo.ID]
	if update != nil && update.Updating == updating {
		heap.Remove(q, update.Index)
		return true
	}

	return false
}

// acquireNext acquires the next repo for update.
// The acquired repo must be removed from the queue
// when the update finishes (independent of success or failure).
func (q *updateQueue) acquireNext() (configuredRepo, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.heap) == 0 {
		return configuredRepo{}, false
	}
	update := q.heap[0]
	if update.Updating {
		// Everything in the queue is already updating.
		return configuredRepo{}, false
	}
	update.Updating = true
	heap.Fix(q, update.Index)
	return update.Repo, true
}

// The following methods implement heap.Interface based on the priority queue example:
// https://golang.org/pkg/container/heap/#example__priorityQueue
// These methods are not safe for concurrent use. Therefore, it is the caller's
// responsibility to ensure they're being guarded by a mutex during any heap operation,
// i.e. heap.Fix, heap.Remove, heap.Push, heap.Pop.

func (q *updateQueue) Len() int {
	n := len(q.heap)
	schedUpdateQueueLength.Set(float64(n))
	return n
}

func (q *updateQueue) Less(i, j int) bool {
	qi := q.heap[i]
	qj := q.heap[j]
	if qi.Updating != qj.Updating {
		// Repos that are already updating are sorted last.
		return qj.Updating
	}
	if qi.Priority != qj.Priority {
		// We want Pop to give us the highest, not lowest, priority so we use greater than here.
		return qi.Priority > qj.Priority
	}
	// Queue semantics for items with the same priority.
	return qi.Seq < qj.Seq
}

func (q *updateQueue) Swap(i, j int) {
	q.heap[i], q.heap[j] = q.heap[j], q.heap[i]
	q.heap[i].Index = i
	q.heap[j].Index = j
}

func (q *updateQueue) Push(x interface{}) {
	n := len(q.heap)
	item := x.(*repoUpdate)
	item.Index = n
	item.Seq = q.nextSeq()
	q.heap = append(q.heap, item)
	q.index[item.Repo.ID] = item
}

func (q *updateQueue) Pop() interface{} {
	n := len(q.heap)
	item := q.heap[n-1]
	item.Index = -1 // for safety
	q.heap = q.heap[0 : n-1]
	delete(q.index, item.Repo.ID)
	return item
}

// schedule is the schedule of when repos get enqueued into the updateQueue.
type schedule struct {
	mu sync.Mutex

	heap  []*scheduledRepoUpdate // min heap of scheduledRepoUpdates based on their due time.
	index map[api.RepoID]*scheduledRepoUpdate

	// timer sends a value on the wakeup channel when it is time
	timer  *time.Timer
	wakeup chan struct{}

	// random source used to add jitter to repo update intervals.
	randGenerator interface {
		Int63n(n int64) int64
	}
}

// scheduledRepoUpdate is the update schedule for a single repo.
type scheduledRepoUpdate struct {
	Repo     configuredRepo // the repo to update
	Interval time.Duration  // how regularly the repo is updated
	Due      time.Time      // the next time that the repo will be enqueued for a update
	Index    int            `json:"-"` // the index in the heap
}

// upsert inserts or updates a repo in the schedule.
func (s *schedule) upsert(repo configuredRepo) (updated bool) {
	if repo.ID == 0 {
		panic("repo.id is zero")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if update := s.index[repo.ID]; update != nil {
		update.Repo = repo
		return true
	}

	heap.Push(s, &scheduledRepoUpdate{
		Repo:     repo,
		Interval: minDelay,
		Due:      timeNow().Add(minDelay),
	})

	s.rescheduleTimer()

	return false
}

func (s *schedule) prioritiseUncloned(names []string) {
	// Set of names created outside of lock for fast checking.
	uncloned := make(map[string]struct{}, len(names))
	for _, n := range names {
		uncloned[strings.ToLower(n)] = struct{}{}
	}

	// All non-cloned repos will be due for cloning as if they are newly added
	// repos.
	notClonedDue := timeNow().Add(minDelay)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Iterate over all repos in the scheduler. If it isn't in cloned bump it
	// up the queue. Note: we iterate over index because we will be mutating
	// heap.
	rescheduleTimer := false
	for _, repoUpdate := range s.index {
		if _, ok := uncloned[strings.ToLower(string(repoUpdate.Repo.Name))]; !ok {
			// It not in the uncloned list, skip
			continue
		}
		if repoUpdate.Due.After(notClonedDue) {
			repoUpdate.Due = notClonedDue
			heap.Fix(s, repoUpdate.Index)
			rescheduleTimer = true
		}
	}

	// We updated the queue, inform the scheduler loop.
	if rescheduleTimer {
		s.rescheduleTimer()
	}
}

// insertNew will insert repos only if they are not known to the scheduler
func (s *schedule) insertNew(repos []types.MinimalRepo) {
	required := make(map[string]struct{}, len(repos))
	for _, n := range repos {
		required[strings.ToLower(string(n.Name))] = struct{}{}
	}

	configuredRepos := make([]configuredRepo, len(repos))
	for i := range repos {
		configuredRepos[i] = configuredRepo{
			ID:   repos[i].ID,
			Name: repos[i].Name,
		}
	}

	due := timeNow().Add(minDelay)
	rescheduleTimer := false

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, repo := range configuredRepos {
		if update := s.index[repo.ID]; update != nil {
			continue
		}
		heap.Push(s, &scheduledRepoUpdate{
			Repo:     repo,
			Interval: minDelay,
			Due:      due,
		})
		rescheduleTimer = true
	}

	if rescheduleTimer {
		s.rescheduleTimer()
	}
}

// updateInterval updates the update interval of a repo in the schedule.
// It does nothing if the repo is not in the schedule.
func (s *schedule) updateInterval(repo configuredRepo, interval time.Duration) {
	if repo.ID == 0 {
		panic("repo.id is zero")
	}

	s.mu.Lock()
	if update := s.index[repo.ID]; update != nil {
		switch {
		case interval > maxDelay:
			update.Interval = maxDelay
		case interval < minDelay:
			update.Interval = minDelay
		default:
			update.Interval = interval
		}

		// Add a jitter of 5% on either side of the interval to avoid
		// repos getting updated at the same time.
		delta := int64(update.Interval) / 20
		update.Interval = update.Interval + time.Duration(s.randGenerator.Int63n(2*delta)-delta)

		update.Due = timeNow().Add(update.Interval)
		log15.Debug("updated repo", "repo", repo.Name, "due", update.Due.Sub(timeNow()))
		heap.Fix(s, update.Index)
		s.rescheduleTimer()
	}
	s.mu.Unlock()
}

// getCurrentInterval gets the current interval for the supplied repo and a bool
// indicating whether it was found.
func (s *schedule) getCurrentInterval(repo configuredRepo) (time.Duration, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	update, ok := s.index[repo.ID]
	if !ok || update == nil {
		return 0, false
	}
	return update.Interval, true
}

// remove removes a repo from the schedule.
func (s *schedule) remove(repo configuredRepo) (removed bool) {
	if repo.ID == 0 {
		panic("repo.id is zero")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	update := s.index[repo.ID]
	if update == nil {
		return false
	}

	reschedule := update.Index == 0
	if heap.Remove(s, update.Index); reschedule {
		s.rescheduleTimer()
	}

	return true
}

// rescheduleTimer schedules the scheduler to wakeup
// at the time that the next repo is due for an update.
// The caller must hold the lock on s.mu.
func (s *schedule) rescheduleTimer() {
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	if len(s.heap) > 0 {
		delay := s.heap[0].Due.Sub(timeNow())
		s.timer = timeAfterFunc(delay, func() {
			notify(s.wakeup)
		})
	}
}

func (s *schedule) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.heap = s.heap[:0]
	s.index = map[api.RepoID]*scheduledRepoUpdate{}
	s.wakeup = make(chan struct{}, notifyChanBuffer)
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}

	log15.Debug("schedKnownRepos reset")
	schedKnownRepos.Set(0)
}

// The following methods implement heap.Interface based on the priority queue example:
// https://golang.org/pkg/container/heap/#example__priorityQueue
// These methods are not safe for concurrent use. Therefore, it is the caller's
// responsibility to ensure they're being guarded by a mutex during any heap operation,
// i.e. heap.Fix, heap.Remove, heap.Push, heap.Pop.

func (s *schedule) Len() int { return len(s.heap) }
func (s *schedule) Less(i, j int) bool {
	return s.heap[i].Due.Before(s.heap[j].Due)
}

func (s *schedule) Swap(i, j int) {
	s.heap[i], s.heap[j] = s.heap[j], s.heap[i]
	s.heap[i].Index = i
	s.heap[j].Index = j
}

func (s *schedule) Push(x interface{}) {
	n := len(s.heap)
	item := x.(*scheduledRepoUpdate)
	item.Index = n
	s.heap = append(s.heap, item)
	s.index[item.Repo.ID] = item
	schedKnownRepos.Inc()
}

func (s *schedule) Pop() interface{} {
	n := len(s.heap)
	item := s.heap[n-1]
	item.Index = -1 // for safety
	s.heap = s.heap[0 : n-1]
	delete(s.index, item.Repo.ID)
	schedKnownRepos.Dec()
	return item
}

// notify performs a non-blocking send on the channel.
// The channel should be buffered.
var notify = func(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// Mockable time functions for testing.
var (
	timeNow       = time.Now
	timeAfterFunc = time.AfterFunc
)

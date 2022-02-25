package main

import (
	"context"
	"strings"

	"github.com/google/go-github/v41/github"
	"github.com/grafana/regexp"
)

type checkResult struct {
	// Reviewed indicates that *any* review has been made on the PR. It is also set to
	// true if the test plan indicates that this PR does not need to be review.
	Reviewed bool
	// TestPlan is the content provided after the acceptance checklist checkbox.
	TestPlan string
	// Error indicating any issue that might have occured during the check.
	Error error
}

func (r checkResult) HasTestPlan() bool {
	return r.TestPlan != ""
}

var (
	testPlanDividerRegexp       = regexp.MustCompile("(?m)(^#+ Test [pP]lan)|(^Test [pP]lan:)")
	noReviewNeededDividerRegexp = regexp.MustCompile("(?m)([nN]o [rR]eview [rR]equired:)")
	markdownCommentRegexp       = regexp.MustCompile("<!--((.|\n)*?)-->(\n)*")
)

type checkOpts struct {
	ValidateReviews bool
}

func checkPR(ctx context.Context, ghc *github.Client, payload *EventPayload, opts checkOpts) checkResult {
	pr := payload.PullRequest

	// Whether or not this PR was reviewed can be inferred from payload, but an approval
	// might not have any comments so we need to double-check through the GitHub API
	var err error
	reviewed := pr.ReviewComments > 0
	if !reviewed && opts.ValidateReviews {
		owner, repo := payload.Repository.GetOwnerAndName()
		var reviews []*github.PullRequestReview
		// Continue, but return err later
		reviews, _, err = ghc.PullRequests.ListReviews(ctx, owner, repo, payload.PullRequest.Number, &github.ListOptions{})
		reviewed = len(reviews) > 0
	}

	// Parse test plan data from body
	sections := testPlanDividerRegexp.Split(pr.Body, 2)
	if len(sections) < 2 {
		return checkResult{
			Reviewed: reviewed,
			Error:    err,
		}
	}

	testPlan := cleanMarkdown(sections[1])

	// Look for no review required explanation in the test plan
	if sections := noReviewNeededDividerRegexp.Split(testPlan, 2); len(sections) > 1 {
		noReviewRequiredExplanation := cleanMarkdown(sections[1])
		if len(noReviewRequiredExplanation) > 0 {
			reviewed = true
		}
	}

	return checkResult{
		Reviewed: reviewed,
		TestPlan: testPlan,
		Error:    err,
	}
}

func cleanMarkdown(s string) string {
	content := s
	// Remove comments
	content = markdownCommentRegexp.ReplaceAllString(content, "")
	// Remove whitespace
	content = strings.TrimSpace(content)

	return content
}

package job

import (
	"bytes"
	"fmt"
	"strconv"

	"github.com/sourcegraph/sourcegraph/internal/search/commit"
	"github.com/sourcegraph/sourcegraph/internal/search/repos"
	"github.com/sourcegraph/sourcegraph/internal/search/run"
	"github.com/sourcegraph/sourcegraph/internal/search/structural"
	"github.com/sourcegraph/sourcegraph/internal/search/symbol"
	"github.com/sourcegraph/sourcegraph/internal/search/textsearch"
)

func writeSep(b *bytes.Buffer, sep, indent string, depth int) {
	b.WriteString(sep)
	if indent == "" {
		return
	}
	for i := 0; i < depth; i++ {
		b.WriteString(indent)
	}
}

// SexpFormat controls the s-expression format that represents a Job. `sep`
// specifies the separator between terms. If `indent` is not empty, `indent` is
// prefixed the number of times corresponding to depth of the term in the tree.
// See the `Sexp` and `PrettySexp` convenience functions to see how these
// options are used.
func SexpFormat(job Job, sep, indent string) string {
	b := new(bytes.Buffer)
	depth := 0
	var writeSexp func(Job)
	writeSexp = func(job Job) {
		switch j := job.(type) {
		case
			*run.RepoSearch,
			*textsearch.RepoSubsetTextSearch,
			*textsearch.RepoUniverseTextSearch,
			*structural.StructuralSearch,
			*commit.CommitSearch,
			*symbol.RepoSubsetSymbolSearch,
			*symbol.RepoUniverseSymbolSearch,
			*repos.ComputeExcludedRepos,
			*noopJob:
			b.WriteString(j.Name())
		case *AndJob:
			b.WriteString("(AND")
			depth++
			for _, child := range j.children {
				writeSep(b, sep, indent, depth)
				writeSexp(child)
			}
			b.WriteString(")")
			depth--
		case *OrJob:
			b.WriteString("(OR")
			depth++
			for _, child := range j.children {
				writeSep(b, sep, indent, depth)
				writeSexp(child)
			}
			b.WriteString(")")
			depth--
		case *PriorityJob:
			b.WriteString("(PRIORITY")
			depth++
			writeSep(b, sep, indent, depth)
			b.WriteString("(REQUIRED")
			depth++
			writeSep(b, sep, indent, depth)
			writeSexp(j.required)
			b.WriteString(")")
			depth--
			writeSep(b, sep, indent, depth)
			b.WriteString("(OPTIONAL")
			depth++
			writeSep(b, sep, indent, depth)
			writeSexp(j.optional)
			b.WriteString(")")
			depth--
			b.WriteString(")")
			depth--
		case *ParallelJob:
			b.WriteString("(PARALLEL")
			depth++
			for _, child := range j.children {
				writeSep(b, sep, indent, depth)
				writeSexp(child)
			}
			depth--
			b.WriteString(")")
		case *TimeoutJob:
			b.WriteString("(TIMEOUT")
			depth++
			writeSep(b, sep, indent, depth)
			b.WriteString(j.timeout.String())
			writeSep(b, sep, indent, depth)
			writeSexp(j.child)
			b.WriteString(")")
			depth--
		case *LimitJob:
			b.WriteString("(LIMIT")
			depth++
			writeSep(b, sep, indent, depth)
			b.WriteString(strconv.Itoa(j.limit))
			writeSep(b, sep, indent, depth)
			writeSexp(j.child)
			b.WriteString(")")
			depth--
		case *subRepoPermsFilterJob:
			b.WriteString("(FILTER")
			depth++
			writeSep(b, sep, indent, depth)
			b.WriteString("SubRepoPermissions")
			writeSep(b, sep, indent, depth)
			writeSexp(j.child)
			b.WriteString(")")
			depth--
		default:
			panic(fmt.Sprintf("unsupported job %T for SexpFormat printer", job))
		}
	}
	writeSexp(job)
	return b.String()
}

// Sexp outputs the s-expression on a single line.
func Sexp(job Job) string {
	return SexpFormat(job, " ", "")
}

// PrettySexp outputs a formatted s-expression with two spaces of indentation, potentially spanning multiple lines.
func PrettySexp(job Job) string {
	return SexpFormat(job, "\n", "  ")
}

func writeEdge(b *bytes.Buffer, src, dst int) {
	b.WriteString(strconv.Itoa(src))
	b.WriteString("-->")
	b.WriteString(strconv.Itoa(dst))
	b.WriteByte('\n')
}

func writeNode(b *bytes.Buffer, id *int, label string) {
	b.WriteString(strconv.Itoa(*id))
	b.WriteByte('[')
	b.WriteString(label)
	b.WriteByte(']')
	b.WriteByte('\n')
	*id++
}

func openSubgraph(b *bytes.Buffer, id *int, label string) {
	if label == "" {
		label = "&nbsp"
	}
	b.WriteString("subgraph ")
	b.WriteString(strconv.Itoa(*id))
	b.WriteByte('[')
	b.WriteString(label)
	b.WriteByte(']')
	b.WriteByte('\n')
	*id++
}

func closeSubgraph(b *bytes.Buffer) {
	b.WriteString("end\n")
}

// PrettyMermaid outputs a Mermaid flowchart. See https://mermaid-js.github.io.
func PrettyMermaid(job Job) string {
	subgraphId := 10000
	id := 0
	b := new(bytes.Buffer)
	b.WriteString("flowchart TB\n")
	var writeMermaid func(Job)
	writeMermaid = func(job Job) {
		switch j := job.(type) {
		case
			*run.RepoSearch,
			*textsearch.RepoSubsetTextSearch,
			*textsearch.RepoUniverseTextSearch,
			*structural.StructuralSearch,
			*commit.CommitSearch,
			*symbol.RepoSubsetSymbolSearch,
			*symbol.RepoUniverseSymbolSearch,
			*repos.ComputeExcludedRepos,
			*noopJob:
			writeNode(b, &id, j.Name())
		case *AndJob:
			openSubgraph(b, &subgraphId, "")
			srcId := id
			writeNode(b, &id, "AND")
			for _, child := range j.children {
				writeEdge(b, srcId, id)
				writeMermaid(child)
			}
			closeSubgraph(b)
		case *OrJob:
			openSubgraph(b, &subgraphId, "")
			srcId := id
			writeNode(b, &id, "OR")
			for _, child := range j.children {
				writeEdge(b, srcId, id)
				writeMermaid(child)
			}
			closeSubgraph(b)
		case *PriorityJob:
			openSubgraph(b, &subgraphId, "")
			srcId := id
			writeNode(b, &id, "PRIORITY")

			requiredId := id
			writeEdge(b, srcId, requiredId)
			writeNode(b, &id, "REQUIRED")
			writeEdge(b, requiredId, id)
			writeMermaid(j.required)

			optionalId := id
			writeEdge(b, srcId, optionalId)
			writeNode(b, &id, "OPTIONAL")
			writeEdge(b, optionalId, id)
			writeMermaid(j.optional)
			closeSubgraph(b)
		case *ParallelJob:
			openSubgraph(b, &subgraphId, "")
			srcId := id
			writeNode(b, &id, "PARALLEL")
			for _, child := range j.children {
				writeEdge(b, srcId, id)
				writeMermaid(child)
			}
			closeSubgraph(b)
		case *TimeoutJob:
			openSubgraph(b, &subgraphId, "")
			srcId := id
			writeNode(b, &id, "TIMEOUT")
			writeEdge(b, srcId, id)
			writeNode(b, &id, j.timeout.String())
			writeEdge(b, srcId, id)
			writeMermaid(j.child)
			closeSubgraph(b)
		case *LimitJob:
			openSubgraph(b, &subgraphId, "")
			srcId := id
			writeNode(b, &id, "LIMIT")
			writeEdge(b, srcId, id)
			writeNode(b, &id, strconv.Itoa(j.limit))
			writeEdge(b, srcId, id)
			writeMermaid(j.child)
			closeSubgraph(b)
		case *subRepoPermsFilterJob:
			openSubgraph(b, &subgraphId, "")
			srcId := id
			writeNode(b, &id, "FILTER")
			writeEdge(b, srcId, id)
			writeNode(b, &id, "SubRepoPermissions")
			writeEdge(b, srcId, id)
			writeMermaid(j.child)
			closeSubgraph(b)
		default:
			panic(fmt.Sprintf("unsupported job %T for SexpFormat printer", job))
		}
	}
	writeMermaid(job)
	return b.String()
}

package reporter

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/MaripeddiSupraj/terrawatch/internal/detector"
)

// driftBranchPrefix marks branches terrawatch created. Auto-close only ever
// touches PRs/MRs whose head branch carries this prefix, so a manually
// created PR that happens to share the title is never closed.
const (
	driftBranchPrefix  = "drift/"
	maxPRPlanBytes     = 45_000
	maxReportPlanBytes = 900_000
)

// safeSlug converts an arbitrary stack name into a single safe path/ref segment.
// User-controlled stack names must never be allowed to create nested paths,
// invalid git refs, or path traversal in drift report filenames.
func safeSlug(input string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range input {
		valid := unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.'
		if valid {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}

	slug := strings.Trim(b.String(), ".-")
	if slug == "" {
		slug = "stack"
	}
	if len(slug) > 64 {
		slug = strings.Trim(slug[:64], ".-")
		if slug == "" {
			slug = "stack"
		}
	}

	// Preserve clean names as-is. If normalization changed the name, append a
	// stable short digest so two different names cannot collapse to one slug.
	if slug != input {
		sum := sha256.Sum256([]byte(input))
		slug = fmt.Sprintf("%s-%x", slug, sum[:4])
	}
	return slug
}

func branchName(stackName string, t time.Time) string {
	return fmt.Sprintf("%s%s-%s", driftBranchPrefix, safeSlug(stackName), t.Format("20060102-150405"))
}

func reportFilename(stackName string, t time.Time) string {
	return fmt.Sprintf("drift-reports/%s-%s.md", safeSlug(stackName), t.Format("20060102-150405"))
}

func prTitle(stackName string) string {
	return fmt.Sprintf("[terrawatch] Drift detected in stack: %s", stackName)
}

func prBody(d detector.DriftResult) string {
	s := d.Plan.Summary
	var b strings.Builder

	b.WriteString("## Terraform Drift Detected\n\n")
	b.WriteString(fmt.Sprintf("**Stack:** `%s`\n", d.Stack.Name))
	b.WriteString(fmt.Sprintf("**Path:** `%s`\n", d.Stack.Path))
	b.WriteString(fmt.Sprintf("**Detected at:** %s\n\n", d.DetectedAt.Format(time.RFC1123)))

	switch d.Kind {
	case detector.KindInfraDrift:
		b.WriteString("> ⚠️ **Real infrastructure drift** — live resources differ from terraform state.\n\n")
	case detector.KindUnappliedChanges:
		b.WriteString("> ℹ️ **Unapplied code changes** — the cloud matches state, but merged code was never applied.\n\n")
	}

	if d.HiddenChanges > 0 {
		b.WriteString(fmt.Sprintf("> **%d change(s) hidden by ignore rules.**\n\n", d.HiddenChanges))
	}

	b.WriteString("### Summary\n\n")
	b.WriteString(fmt.Sprintf("| Add | Change | Destroy |\n|-----|--------|---------|\n| %d | %d | %d |\n\n", s.Add, s.Change, s.Destroy))

	b.WriteString("### Plan\n\n")
	b.WriteString("<details>\n<summary>Click to expand diff</summary>\n\n")
	b.WriteString("```diff\n")
	plan, truncated := truncatePlan(planAsDiff(d.Plan.Output), maxPRPlanBytes)
	b.WriteString(plan)
	b.WriteString("\n```\n\n")
	if truncated {
		b.WriteString("> Plan output was truncated in the PR body. The committed drift report contains a larger excerpt.\n\n")
	}
	b.WriteString("</details>\n\n")

	b.WriteString("---\n")
	b.WriteString("_Auto-detected by [terrawatch](https://github.com/MaripeddiSupraj/terrawatch). Review and apply to resolve drift._\n")

	return b.String()
}

func reportFileContent(d detector.DriftResult) string {
	body := prBody(d)
	if len(d.Plan.Output) <= maxPRPlanBytes {
		return body
	}

	// The PR body is intentionally small enough for VCS APIs. The committed
	// report carries a larger bounded excerpt without risking the GitHub
	// Contents API size limit.
	fullPlan, truncated := truncatePlan(planAsDiff(d.Plan.Output), maxReportPlanBytes)
	start := strings.Index(body, "```diff\n")
	if start == -1 {
		return body
	}
	contentStart := start + len("```diff\n")
	end := strings.Index(body[contentStart:], "\n```")
	if end == -1 {
		return body
	}
	contentEnd := contentStart + end
	replacement := fullPlan
	if truncated {
		replacement += "\n\n# ... output truncated by terrawatch ..."
	}
	return body[:contentStart] + replacement + body[contentEnd:]
}

func truncatePlan(plan string, limit int) (string, bool) {
	if limit <= 0 || len(plan) <= limit {
		return plan, false
	}
	cut := limit
	for cut > 0 && cut < len(plan) && (plan[cut]&0xC0) == 0x80 {
		cut--
	}
	return plan[:cut] + "\n... output truncated by terrawatch ...", true
}

// planAsDiff maps terraform plan symbols so GitHub diff syntax highlights them:
//
//	lines starting with "+" → green
//	lines starting with "-" → red
//	lines starting with "~" → prefixed with "-/+" so both colors appear
func planAsDiff(output string) string {
	lines := strings.Split(output, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " ")
		switch {
		case strings.HasPrefix(trimmed, "+ ") || strings.HasPrefix(trimmed, "+\""):
			out = append(out, line)
		case strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "-\""):
			out = append(out, line)
		case strings.HasPrefix(trimmed, "~ "):
			out = append(out, "- "+strings.TrimPrefix(trimmed, "~ "))
			out = append(out, "+ "+strings.TrimPrefix(trimmed, "~ "))
		default:
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

// resolvedCommentBody is posted to a drift PR just before auto-closing it.
func resolvedCommentBody(stackName string, t time.Time) string {
	return fmt.Sprintf(
		"### ✅ Drift resolved\n\nterrawatch re-checked **%s** at %s and found no drift — closing this PR automatically.\n",
		stackName, t.Format(time.RFC1123))
}

// commentBody builds the comment posted to an existing drift PR on re-detection.
func commentBody(d detector.DriftResult) string {
	s := d.Plan.Summary
	var b strings.Builder

	b.WriteString("### Drift still present\n\n")
	b.WriteString(fmt.Sprintf("terrawatch re-checked **%s** at %s and drift is still detected.\n\n",
		d.Stack.Name, d.DetectedAt.Format(time.RFC1123)))

	if d.HiddenChanges > 0 {
		b.WriteString(fmt.Sprintf("> **%d change(s) hidden by ignore rules.**\n\n", d.HiddenChanges))
	}

	b.WriteString(fmt.Sprintf("| Add | Change | Destroy |\n|-----|--------|---------|\n| %d | %d | %d |\n\n",
		s.Add, s.Change, s.Destroy))

	b.WriteString("<details>\n<summary>Updated plan diff</summary>\n\n")
	b.WriteString("```diff\n")
	b.WriteString(planAsDiff(d.Plan.Output))
	b.WriteString("\n```\n</details>\n")

	return b.String()
}

package reporter

import (
	"fmt"
	"strings"
	"time"

	"github.com/MaripeddiSupraj/terrawatch/internal/detector"
)

func branchName(stackName string, t time.Time) string {
	return fmt.Sprintf("drift/%s-%s", stackName, t.Format("20060102-150405"))
}

func reportFilename(stackName string, t time.Time) string {
	return fmt.Sprintf("drift-reports/%s-%s.md", stackName, t.Format("20060102-150405"))
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

	b.WriteString("### Summary\n\n")
	b.WriteString(fmt.Sprintf("| Add | Change | Destroy |\n|-----|--------|---------|\n| %d | %d | %d |\n\n", s.Add, s.Change, s.Destroy))

	b.WriteString("### Plan\n\n")
	b.WriteString("<details>\n<summary>Click to expand full diff</summary>\n\n")
	b.WriteString("```diff\n")
	b.WriteString(planAsDiff(d.Plan.Output))
	b.WriteString("\n```\n\n</details>\n\n")

	b.WriteString("---\n")
	b.WriteString("_Auto-detected by [terrawatch](https://github.com/MaripeddiSupraj/terrawatch). Review and apply to resolve drift._\n")

	return b.String()
}

func reportFileContent(d detector.DriftResult) string {
	return prBody(d)
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
			// show update lines as removed-then-added so diff coloring makes sense
			out = append(out, "- "+strings.TrimPrefix(trimmed, "~ "))
			out = append(out, "+ "+strings.TrimPrefix(trimmed, "~ "))
		default:
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

// commentBody builds the comment posted to an existing drift PR on re-detection.
func commentBody(d detector.DriftResult) string {
	s := d.Plan.Summary
	var b strings.Builder

	b.WriteString("### Drift still present\n\n")
	b.WriteString(fmt.Sprintf("terrawatch re-checked **%s** at %s and drift is still detected.\n\n",
		d.Stack.Name, d.DetectedAt.Format(time.RFC1123)))

	b.WriteString(fmt.Sprintf("| Add | Change | Destroy |\n|-----|--------|---------|\n| %d | %d | %d |\n\n",
		s.Add, s.Change, s.Destroy))

	b.WriteString("<details>\n<summary>Updated plan diff</summary>\n\n")
	b.WriteString("```diff\n")
	b.WriteString(planAsDiff(d.Plan.Output))
	b.WriteString("\n```\n\n</details>\n")

	return b.String()
}

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
	b.WriteString("<details>\n<summary>Click to expand</summary>\n\n")
	b.WriteString("```hcl\n")
	b.WriteString(d.Plan.Output)
	b.WriteString("\n```\n\n</details>\n\n")

	b.WriteString("---\n")
	b.WriteString("_Auto-detected by [terrawatch](https://github.com/MaripeddiSupraj/terrawatch). Review and apply to resolve drift._\n")

	return b.String()
}

func reportFileContent(d detector.DriftResult) string {
	return prBody(d)
}

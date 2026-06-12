package driftfilter

import (
	"encoding/json"
	"path"
	"reflect"
	"strings"

	"github.com/MaripeddiSupraj/terrawatch/internal/config"
	"github.com/MaripeddiSupraj/terrawatch/pkg/terraform"
)

// Result holds the output of filtering resource changes through ignore rules.
type Result struct {
	Changes       []terraform.ResourceChange
	HiddenChanges int
}

// Apply filters a set of resource changes against global and per-stack ignore rules.
// Rules are merged: stack rules are appended to global rules. All rules matching a
// resource combine — a whole-resource rule (no attributes) drops it outright, and
// attribute rules contribute the union of their ignored attribute paths.
func Apply(changes []terraform.ResourceChange, globalRules, stackRules []config.IgnoreRule) Result {
	// copy to avoid appending into globalRules' backing array
	rules := make([]config.IgnoreRule, 0, len(globalRules)+len(stackRules))
	rules = append(rules, globalRules...)
	rules = append(rules, stackRules...)
	if len(rules) == 0 {
		return Result{Changes: changes}
	}

	var filtered []terraform.ResourceChange
	hidden := 0

	for _, ch := range changes {
		ignoreWhole, attrs := matchRules(ch.Address, rules)

		if ignoreWhole {
			hidden += countActions(ch.Actions)
			continue
		}

		if len(attrs) == 0 {
			filtered = append(filtered, ch)
			continue
		}

		// Check if removing the ignored attributes makes before == after.
		before := deepCopyMap(ch.Before)
		after := deepCopyMap(ch.After)

		for _, attr := range attrs {
			deleteNested(before, attr)
			deleteNested(after, attr)
		}

		if reflect.DeepEqual(before, after) {
			hidden += countActions(ch.Actions)
			continue
		}

		filtered = append(filtered, ch)
	}

	return Result{Changes: filtered, HiddenChanges: hidden}
}

// ComputeSummary builds a Summary from a set of resource changes.
func ComputeSummary(changes []terraform.ResourceChange) terraform.Summary {
	var s terraform.Summary
	for _, ch := range changes {
		for _, a := range ch.Actions {
			switch a {
			case "create":
				s.Add++
			case "update":
				s.Change++
			case "delete":
				s.Destroy++
			}
		}
	}
	return s
}

// matchRules evaluates every rule against the address. It returns
// ignoreWhole=true if any matching rule has no attributes, otherwise the
// union of attribute paths from all matching rules.
func matchRules(address string, rules []config.IgnoreRule) (ignoreWhole bool, attrs []string) {
	seen := map[string]bool{}
	for _, r := range rules {
		ok, err := path.Match(r.Resource, address)
		if err != nil || !ok {
			continue
		}
		if len(r.Attributes) == 0 {
			return true, nil
		}
		for _, a := range r.Attributes {
			if !seen[a] {
				seen[a] = true
				attrs = append(attrs, a)
			}
		}
	}
	return false, attrs
}

func countActions(actions []string) int {
	n := 0
	for _, a := range actions {
		if a == "create" || a == "update" || a == "delete" {
			n++
		}
	}
	return n
}

// deleteNested removes a value from a nested map using a dot-separated path.
// e.g., "tags.Name" deletes m["tags"]["Name"].
// It does nothing if any intermediate key does not exist or is not a map.
func deleteNested(m map[string]any, dotPath string) {
	parts := strings.SplitN(dotPath, ".", 2)
	if len(parts) == 1 {
		delete(m, parts[0])
		return
	}
	if nested, ok := m[parts[0]].(map[string]any); ok {
		deleteNested(nested, parts[1])
	}
}

// deepCopyMap deep-copies a map via JSON round-trip.
func deepCopyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return m
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return m
	}
	return out
}

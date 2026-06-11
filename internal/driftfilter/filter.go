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
// Rules are merged: stack rules are appended to global rules.
func Apply(changes []terraform.ResourceChange, globalRules, stackRules []config.IgnoreRule) Result {
	rules := append(globalRules, stackRules...)
	if len(rules) == 0 {
		return Result{Changes: changes}
	}

	var filtered []terraform.ResourceChange
	hidden := 0

	for _, ch := range changes {
		rule, matched := matchRule(ch.Address, rules)
		if !matched {
			filtered = append(filtered, ch)
			continue
		}

		if len(rule.Attributes) == 0 {
			hidden += countActions(ch.Actions)
			continue
		}

		// Check if removing the specified attributes makes before == after.
		before := deepCopyMap(ch.Before)
		after := deepCopyMap(ch.After)

		for _, attr := range rule.Attributes {
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

func matchRule(address string, rules []config.IgnoreRule) (config.IgnoreRule, bool) {
	for _, r := range rules {
		if ok, _ := path.Match(r.Resource, address); ok {
			return r, true
		}
	}
	return config.IgnoreRule{}, false
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

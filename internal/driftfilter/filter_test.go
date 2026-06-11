package driftfilter

import (
	"testing"

	"github.com/MaripeddiSupraj/terrawatch/internal/config"
	"github.com/MaripeddiSupraj/terrawatch/pkg/terraform"
)

func mkChange(addr string, actions []string, before, after map[string]any) terraform.ResourceChange {
	return terraform.ResourceChange{
		Address: addr,
		Actions: actions,
		Before:  before,
		After:   after,
	}
}

func mkRule(resource string, attrs ...string) config.IgnoreRule {
	return config.IgnoreRule{Resource: resource, Attributes: attrs}
}

func TestApply_no_rules(t *testing.T) {
	changes := []terraform.ResourceChange{
		mkChange("aws_instance.web", []string{"update"}, nil, nil),
	}
	result := Apply(changes, nil, nil)
	if len(result.Changes) != 1 {
		t.Errorf("expected 1 change, got %d", len(result.Changes))
	}
	if result.HiddenChanges != 0 {
		t.Errorf("expected 0 hidden, got %d", result.HiddenChanges)
	}
}

func TestApply_whole_resource_ignore(t *testing.T) {
	changes := []terraform.ResourceChange{
		mkChange("null_resource.ephemeral", []string{"create"}, nil, nil),
	}
	rules := []config.IgnoreRule{mkRule("null_resource.ephemeral")}
	result := Apply(changes, rules, nil)

	if len(result.Changes) != 0 {
		t.Errorf("expected 0 changes, got %d", len(result.Changes))
	}
	if result.HiddenChanges != 1 {
		t.Errorf("expected 1 hidden, got %d", result.HiddenChanges)
	}
}

func TestApply_glob_pattern(t *testing.T) {
	changes := []terraform.ResourceChange{
		mkChange("aws_autoscaling_group.web", []string{"update"}, nil, nil),
		mkChange("aws_instance.web", []string{"update"}, nil, nil),
	}
	rules := []config.IgnoreRule{mkRule("aws_autoscaling_group.*")}
	result := Apply(changes, rules, nil)

	if len(result.Changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(result.Changes))
	}
	if result.Changes[0].Address != "aws_instance.web" {
		t.Errorf("expected aws_instance.web to remain, got %s", result.Changes[0].Address)
	}
	if result.HiddenChanges != 1 {
		t.Errorf("expected 1 hidden, got %d", result.HiddenChanges)
	}
}

func TestApply_glob_prefix(t *testing.T) {
	changes := []terraform.ResourceChange{
		mkChange("aws_iam_role.admin", []string{"update"}, nil, nil),
		mkChange("aws_iam_policy.readonly", []string{"update"}, nil, nil),
	}
	rules := []config.IgnoreRule{mkRule("aws_iam_*")}
	result := Apply(changes, rules, nil)

	if len(result.Changes) != 0 {
		t.Errorf("expected 0 changes, got %d", len(result.Changes))
	}
	if result.HiddenChanges != 2 {
		t.Errorf("expected 2 hidden, got %d", result.HiddenChanges)
	}
}

func TestApply_glob_no_match(t *testing.T) {
	changes := []terraform.ResourceChange{
		mkChange("aws_s3_bucket.data", []string{"update"}, nil, nil),
	}
	rules := []config.IgnoreRule{mkRule("aws_instance.*")}
	result := Apply(changes, rules, nil)

	if len(result.Changes) != 1 {
		t.Errorf("expected 1 change (no match), got %d", len(result.Changes))
	}
	if result.HiddenChanges != 0 {
		t.Errorf("expected 0 hidden, got %d", result.HiddenChanges)
	}
}

func TestApply_attribute_ignored_resource_clean(t *testing.T) {
	before := map[string]any{"desired_capacity": 3, "name": "web"}
	after := map[string]any{"desired_capacity": 5, "name": "web"}
	changes := []terraform.ResourceChange{
		mkChange("aws_autoscaling_group.web", []string{"update"}, before, after),
	}
	rules := []config.IgnoreRule{
		{Resource: "aws_autoscaling_group.*", Attributes: []string{"desired_capacity"}},
	}
	result := Apply(changes, rules, nil)

	// Only desired_capacity differs; after removing it, before == after
	if len(result.Changes) != 0 {
		t.Errorf("expected 0 changes (all attrs ignored), got %d", len(result.Changes))
	}
	if result.HiddenChanges != 1 {
		t.Errorf("expected 1 hidden, got %d", result.HiddenChanges)
	}
}

func TestApply_attribute_ignored_resource_still_drifted(t *testing.T) {
	before := map[string]any{"desired_capacity": 3, "name": "web", "instance_type": "t3.small"}
	after := map[string]any{"desired_capacity": 5, "name": "web", "instance_type": "t3.large"}
	changes := []terraform.ResourceChange{
		mkChange("aws_autoscaling_group.web", []string{"update"}, before, after),
	}
	rules := []config.IgnoreRule{
		{Resource: "aws_autoscaling_group.*", Attributes: []string{"desired_capacity"}},
	}
	result := Apply(changes, rules, nil)

	// instance_type still differs, so resource should remain
	if len(result.Changes) != 1 {
		t.Errorf("expected 1 change (instance_type still differs), got %d", len(result.Changes))
	}
	if result.HiddenChanges != 0 {
		t.Errorf("expected 0 hidden (resource still drifted), got %d", result.HiddenChanges)
	}
}

func TestApply_nested_attribute_ignored(t *testing.T) {
	before := map[string]any{
		"name": "web",
		"tags": map[string]any{"Name": "web", "LastScanned": "2024-01-01"},
	}
	after := map[string]any{
		"name": "web",
		"tags": map[string]any{"Name": "web", "LastScanned": "2024-06-01"},
	}
	changes := []terraform.ResourceChange{
		mkChange("aws_instance.web", []string{"update"}, before, after),
	}
	rules := []config.IgnoreRule{
		{Resource: "*", Attributes: []string{"tags.LastScanned"}},
	}
	result := Apply(changes, rules, nil)

	// Only tags.LastScanned differs; after removing it, before == after
	if len(result.Changes) != 0 {
		t.Errorf("expected 0 changes (nested attr ignored), got %d", len(result.Changes))
	}
	if result.HiddenChanges != 1 {
		t.Errorf("expected 1 hidden, got %d", result.HiddenChanges)
	}
}

func TestApply_per_stack_merge_with_global(t *testing.T) {
	global := []config.IgnoreRule{mkRule("aws_autoscaling_group.*")}
	stack := []config.IgnoreRule{mkRule("aws_instance.canary")}

	changes := []terraform.ResourceChange{
		mkChange("aws_autoscaling_group.web", []string{"update"}, nil, nil),
		mkChange("aws_instance.canary", []string{"update"}, nil, nil),
		mkChange("aws_instance.web", []string{"update"}, nil, nil),
	}
	result := Apply(changes, global, stack)

	if len(result.Changes) != 1 {
		t.Fatalf("expected 1 change (aws_instance.web), got %d", len(result.Changes))
	}
	if result.Changes[0].Address != "aws_instance.web" {
		t.Errorf("expected aws_instance.web, got %s", result.Changes[0].Address)
	}
	if result.HiddenChanges != 2 {
		t.Errorf("expected 2 hidden, got %d", result.HiddenChanges)
	}
}

func TestApply_replacement_resource(t *testing.T) {
	// A replacement has both delete and create actions
	changes := []terraform.ResourceChange{
		mkChange("null_resource.ephemeral", []string{"delete", "create"}, nil, nil),
	}
	rules := []config.IgnoreRule{mkRule("null_resource.*")}
	result := Apply(changes, rules, nil)

	if len(result.Changes) != 0 {
		t.Errorf("expected 0 changes, got %d", len(result.Changes))
	}
	// Both delete and create count
	if result.HiddenChanges != 2 {
		t.Errorf("expected 2 hidden (delete+create), got %d", result.HiddenChanges)
	}
}

func TestApply_noop_action_not_counted(t *testing.T) {
	changes := []terraform.ResourceChange{
		mkChange("null_resource.foo", []string{"no-op"}, nil, nil),
	}
	rules := []config.IgnoreRule{mkRule("null_resource.*")}
	result := Apply(changes, rules, nil)

	if result.HiddenChanges != 0 {
		t.Errorf("expected 0 hidden (no-op not counted), got %d", result.HiddenChanges)
	}
}

func TestComputeSummary(t *testing.T) {
	changes := []terraform.ResourceChange{
		mkChange("a", []string{"create"}, nil, nil),
		mkChange("b", []string{"update"}, nil, nil),
		mkChange("c", []string{"delete"}, nil, nil),
		mkChange("d", []string{"delete", "create"}, nil, nil),
	}
	s := ComputeSummary(changes)
	if s.Add != 2 {
		t.Errorf("expected Add=2 (c+d), got %d", s.Add)
	}
	if s.Change != 1 {
		t.Errorf("expected Change=1 (b), got %d", s.Change)
	}
	if s.Destroy != 2 {
		t.Errorf("expected Destroy=2 (c+d), got %d", s.Destroy)
	}
}

func TestDeleteNested_simple_key(t *testing.T) {
	m := map[string]any{"a": 1, "b": 2}
	deleteNested(m, "a")
	if _, ok := m["a"]; ok {
		t.Error("expected key 'a' to be deleted")
	}
	if m["b"] != 2 {
		t.Error("expected key 'b' to remain")
	}
}

func TestDeleteNested_dot_path(t *testing.T) {
	m := map[string]any{
		"tags": map[string]any{"Name": "web", "Env": "prod"},
	}
	deleteNested(m, "tags.Name")
	tags := m["tags"].(map[string]any)
	if _, ok := tags["Name"]; ok {
		t.Error("expected 'tags.Name' to be deleted")
	}
	if tags["Env"] != "prod" {
		t.Error("expected 'tags.Env' to remain")
	}
}

func TestDeleteNested_missing_intermediate(t *testing.T) {
	m := map[string]any{"a": 1}
	// should not panic
	deleteNested(m, "b.c")
}

func TestMatchRule_no_match(t *testing.T) {
	rules := []config.IgnoreRule{mkRule("aws_instance.*")}
	_, matched := matchRule("aws_s3_bucket.data", rules)
	if matched {
		t.Error("expected no match")
	}
}

func TestMatchRule_exact_match(t *testing.T) {
	rules := []config.IgnoreRule{mkRule("aws_instance.web")}
	_, matched := matchRule("aws_instance.web", rules)
	if !matched {
		t.Error("expected match")
	}
}

func TestMatchRule_uses_first_rule(t *testing.T) {
	rules := []config.IgnoreRule{
		mkRule("aws_instance.web", "attr1"),
		mkRule("*", "attr2"),
	}
	rule, matched := matchRule("aws_instance.web", rules)
	if !matched {
		t.Fatal("expected match")
	}
	if len(rule.Attributes) != 1 || rule.Attributes[0] != "attr1" {
		t.Errorf("expected first rule's attrs, got %v", rule.Attributes)
	}
}

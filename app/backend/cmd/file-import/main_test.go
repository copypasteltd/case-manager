package main

import (
	"bytes"
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/fileimport"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportCommandWorkflow(t *testing.T) {
	root := t.TempDir()
	out := t.TempDir()
	os.WriteFile(filepath.Join(root, "原件.txt"), []byte("synthetic CLI input\n"), 0600)
	inv, template, mapping, plan := filepath.Join(out, "inventory.jsonl"), filepath.Join(out, "template.jsonl"), filepath.Join(out, "mapping.jsonl"), filepath.Join(out, "plan.jsonl")
	run := func(args ...string) error {
		t.Helper()
		var stdout, stderr bytes.Buffer
		return execute(context.Background(), args, &stdout, &stderr)
	}
	if err := run("inventory", "--root", root, "--namespace", core.NewID(), "--out", inv); err != nil {
		t.Fatal(err)
	}
	if err := run("template", "--inventory", inv, "--out", template); err != nil {
		t.Fatal(err)
	}
	if err := run("plan", "--inventory", inv, "--mapping", template, "--reason", "unreviewed template", "--out", plan); err == nil {
		t.Fatal("unreviewed template became plan")
	}
	raw, _ := os.ReadFile(template)
	var m fileimport.Mapping
	json.Unmarshal(bytes.TrimSpace(raw), &m)
	m.SourceOwner = "reviewed synthetic owner"
	m.MappedOwnerID = core.NewID()
	m.FolderID = core.NewID()
	m.FolderVersion = 1
	m.Classification = "internal"
	raw, _ = json.Marshal(m)
	os.WriteFile(mapping, append(raw, '\n'), 0600)
	if err := run("plan", "--inventory", inv, "--mapping", mapping, "--reason", "reviewed synthetic source", "--out", plan); err != nil {
		t.Fatal(err)
	}
	saved, _ := os.ReadFile(plan)
	if _, err := fileimport.ReadPlan(bytes.NewReader(saved), nil); err != nil {
		t.Fatal(err)
	}
	if err := run("inventory", "--root", root, "--namespace", core.NewID(), "--out", inv); err == nil {
		t.Fatal("saved inventory overwritten")
	}
	if code := safeCode(os.ErrPermission); strings.Contains(code, " ") {
		t.Fatal("arbitrary error printed")
	}
	if err := run("report", "--api", "https://api.invalid/v1", "--batch", core.NewID(), "--out", filepath.Join(out, "missing-token.jsonl")); err == nil {
		t.Fatal("missing session accepted")
	}
}

package vm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAllocateVsockCID(t *testing.T) {
	// Build a fake VMs directory tree and verify the allocator skips
	// already-assigned CIDs and returns the next free one.
	baseDir := t.TempDir()
	vmsDir := filepath.Join(baseDir, "vms")
	if err := os.MkdirAll(vmsDir, 0755); err != nil {
		t.Fatalf("mkdir vmsDir: %v", err)
	}

	writeState := func(name string, cid uint32) {
		dir := filepath.Join(vmsDir, name)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		s := StateFile{Name: name, VsockCID: cid, Backend: "qemu"}
		data, _ := json.Marshal(s)
		if err := os.WriteFile(filepath.Join(dir, "state.json"), data, 0644); err != nil {
			t.Fatalf("write state: %v", err)
		}
	}

	// No VMs yet — should pick the first free CID (3).
	cid, err := allocateVsockCID(baseDir)
	if err != nil {
		t.Fatalf("allocate empty: %v", err)
	}
	if cid != 3 {
		t.Errorf("empty dir: got CID %d, want 3", cid)
	}

	// Pre-seed some VMs taking CIDs 3, 4, 6.
	writeState("alpha", 3)
	writeState("beta", 4)
	writeState("gamma", 6)

	cid, err = allocateVsockCID(baseDir)
	if err != nil {
		t.Fatalf("allocate with holes: %v", err)
	}
	// First free CID is 5 (since 3, 4 are used; 5 is free; 6 is used).
	if cid != 5 {
		t.Errorf("with holes: got CID %d, want 5", cid)
	}

	// VMs without CID set (VsockCID == 0) should be ignored.
	writeState("no-cid", 0)
	cid2, err := allocateVsockCID(baseDir)
	if err != nil {
		t.Fatalf("allocate with no-cid entry: %v", err)
	}
	if cid2 != 5 {
		t.Errorf("no-cid entry must be ignored: got %d, want 5", cid2)
	}
}

func TestAllocateVsockCIDMissingDir(t *testing.T) {
	// baseDir without a vms/ subdir should still return the first CID.
	baseDir := t.TempDir()
	cid, err := allocateVsockCID(baseDir)
	if err != nil {
		t.Fatalf("missing vms dir: %v", err)
	}
	if cid != 3 {
		t.Errorf("missing dir: got CID %d, want 3", cid)
	}
}

func TestClearVsockCIDOnStop(t *testing.T) {
	writeStateWithCID := func(t *testing.T, dir string, cid uint32) {
		t.Helper()
		s := StateFile{Name: filepath.Base(dir), VsockCID: cid, Backend: "qemu"}
		data, _ := json.Marshal(s)
		if err := os.WriteFile(filepath.Join(dir, "state.json"), data, 0644); err != nil {
			t.Fatalf("write state: %v", err)
		}
	}

	t.Run("clears CID when stopped", func(t *testing.T) {
		dir := t.TempDir()
		writeStateWithCID(t, dir, 7)
		inst := &Instance{Dir: dir, VsockCID: 7, VMState: StateStopped}

		clearVsockCIDOnStop(inst)

		if inst.VsockCID != 0 {
			t.Errorf("inst.VsockCID = %d, want 0", inst.VsockCID)
		}
		state, err := loadState(dir)
		if err != nil {
			t.Fatalf("loadState: %v", err)
		}
		if state.VsockCID != 0 {
			t.Errorf("state.VsockCID = %d, want 0", state.VsockCID)
		}
	})

	t.Run("noop when not stopped", func(t *testing.T) {
		dir := t.TempDir()
		writeStateWithCID(t, dir, 7)
		inst := &Instance{Dir: dir, VsockCID: 7, VMState: StateRunning}

		clearVsockCIDOnStop(inst)

		state, _ := loadState(dir)
		if state.VsockCID != 7 || inst.VsockCID != 7 {
			t.Errorf("CID changed while running: state=%d inst=%d", state.VsockCID, inst.VsockCID)
		}
	})

	t.Run("noop when CID already zero", func(t *testing.T) {
		dir := t.TempDir()
		writeStateWithCID(t, dir, 0)
		inst := &Instance{Dir: dir, VsockCID: 0, VMState: StateStopped}

		clearVsockCIDOnStop(inst) // must not error or panic on missing/zero CID
	})

	t.Run("noop when state file missing", func(t *testing.T) {
		dir := t.TempDir()
		inst := &Instance{Dir: dir, VsockCID: 5, VMState: StateStopped}

		clearVsockCIDOnStop(inst) // tolerated — best-effort cleanup
	})
}

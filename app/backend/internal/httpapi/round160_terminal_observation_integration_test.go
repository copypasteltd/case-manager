package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/core"
	storage "copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/objectstore"
	"strings"
	"testing"
)

type round160Head struct {
	cloud   *originalCloud
	calls   int
	missing bool
	after   func()
}

func (c *round160Head) Head(_ context.Context, key string) (objectstore.Source, error) {
	c.calls++
	if c.after != nil {
		c.after()
		c.after = nil
	}
	if c.missing {
		return objectstore.Source{}, &objectstore.Error{Code: "OBJECT_NOT_FOUND"}
	}
	v, ok := c.cloud.sources[key]
	if !ok {
		return v, &objectstore.Error{Code: "OBJECT_NOT_FOUND"}
	}
	return v, nil
}
func TestRound160TerminalObservation(t *testing.T) {
	for _, test := range []string{"all known coordinates become a fresh delayed retirement basis", "HEAD404 never proves absence or enables cleanup", "revoked original human authority stops further I/O"} {
		t.Run(test, func(t *testing.T) {
			f, u := completedUploadFixture(t, false, "android")
			ctx := context.Background()
			observer := &round160Head{cloud: f.cloud, missing: strings.HasPrefix(test, "HEAD404")}
			for _, b := range f.service.Backends {
				b.Observer = observer
			}
			f.f.f.api.OriginalCleanup = f.service.Backends
			f.f.f.handler = f.f.f.api.Handler()
			get := func() storage.TerminalOverview {
				t.Helper()
				r, v := f.request("GET", "/v1/uploads/"+u.ID+"/terminal-observations", "", nil)
				expect(t, r, 200, v)
				if strings.Contains(r.Body.String(), "fixture-bucket") || strings.Contains(r.Body.String(), "synthetic-completed-native") {
					t.Fatal("private coordinates leaked")
				}
				return data[storage.TerminalOverview](t, v)
			}
			candidate := get().Candidate
			if !candidate.CanObserve || candidate.ObjectCount != 2 {
				t.Fatal(candidate)
			}
			in := storage.TerminalObserveCreate{ExpectedFolderVersion: candidate.FolderVersion, ExpectedUploadVersion: candidate.UploadVersion, BasisSHA256: candidate.BasisSHA256, Reason: "review every exact recorded coordinate; no mutation", Reviewed: true}
			r, v := f.request("POST", "/v1/uploads/"+u.ID+"/terminal-observations", core.NewID(), in)
			expect(t, r, 201, v)
			request := data[storage.TerminalObservation](t, v)
			if request.Outcome != "pending" || request.AllPresent {
				t.Fatal(request)
			}
			if strings.HasPrefix(test, "revoked") {
				observer.after = func() {
					if _, err := f.f.f.owner.Exec(ctx, "DELETE FROM role_bindings WHERE org_id=$1 AND user_id=$2 AND role_id IN(SELECT id FROM roles WHERE org_id=$1 AND key='file_retention_manager')", f.f.f.org, f.f.f.alice); err != nil {
						t.Fatal(err)
					}
				}
			}
			worked, err := f.service.RunTerminalOne(ctx, "round160.head-only")
			if err != nil || !worked {
				t.Fatal(worked, err)
			}
			var outcome string
			var all bool
			if err = f.f.f.owner.QueryRow(ctx, "SELECT outcome,all_present FROM terminal_upload_observation_results WHERE request_id=$1", request.ID).Scan(&outcome, &all); err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(test, "revoked") {
				if outcome != "stopped" || all || observer.calls != 1 {
					t.Fatal(outcome, all, observer.calls)
				}
				return
			}
			if observer.calls != 2 || f.cloud.calls != 0 {
				t.Fatal("unexpected non-HEAD or incomplete inspection", observer.calls, f.cloud.calls)
			}
			if observer.missing {
				if outcome != "incomplete" || all {
					t.Fatal(outcome, all)
				}
			} else {
				if outcome != "observed" || !all {
					t.Fatal(outcome, all)
				}
				c := completedCandidates(t, f)
				found := false
				for _, v := range c {
					if v.TargetID == u.ID {
						found = true
						if !strings.Contains(strings.Join(v.BlockingReasons, ","), "RETENTION_PERIOD_ACTIVE") {
							t.Fatal("new observation must restart retention delay", v)
						}
					}
				}
				if !found {
					t.Fatal("observed candidate missing")
				}
			}
			if _, err = f.f.f.owner.Exec(ctx, "UPDATE terminal_upload_observation_results SET all_present=false WHERE request_id=$1", request.ID); err == nil {
				t.Fatal("immutable result changed")
			}
			current := f.f.get(u.ID)
			if current.State != u.State || current.Version != u.Version {
				t.Fatal("HEAD rewrote upload business state", current)
			}
		})
	}
}

package httpapi

import (
	"context"
	"copypaste.hk/platform/internal/core"
	"copypaste.hk/platform/internal/objectstore"
	"copypaste.hk/platform/internal/project"
	"fmt"
	"os"
	"testing"
	"time"

	storage "copypaste.hk/platform/internal/files"
	"copypaste.hk/platform/internal/jobqueue"
	"copypaste.hk/platform/internal/notification"
)

// Explicitly owned synthetic fixture; real HTTP writes, native PostgreSQL and
// durable leased notification jobs. This never starts an external sender.
func TestTaskMentionsBrowserFixture(t *testing.T) {
	f := newUploadFixture(t)
	configureStorageTargets(f)
	allowMobileContent(f)
	first, firstCloud := commitContentFixture(t, f, "任务讨论固定依据.txt", "text/plain", []byte("香港项目 · 任务讨论固定版本一\n请核对明确提及与不可变历史。"), nil)
	second, secondCloud := commitContentFixture(t, f, "任务讨论后续依据.txt", "text/plain", []byte("版本二不能代替评论关联的版本一。"), &first)
	readers := map[string]*contentFixtureCloud{}
	for key := range firstCloud.stored {
		readers[key] = firstCloud
	}
	for key := range secondCloud.stored {
		readers[key] = secondCloud
	}
	cloud := &downloadCloud{readFn: func(ctx context.Context, source objectstore.Source, offset, length int64) (objectstore.ReadResult, error) {
		reader, ok := readers[source.Key]
		if !ok {
			return objectstore.ReadResult{}, fmt.Errorf("unknown owned fixture object")
		}
		return reader.Read(ctx, source, offset, length)
	}}
	f.f.api.Downloads = map[string]*storage.DownloadBackend{"oss": {Provider: "oss", Bucket: "fixture-bucket", Signer: cloud, Reader: cloud}, "r2": {Provider: "r2", Bucket: "fixture-r2-bucket", Signer: cloud, Reader: cloud}}
	p := project.Project{ID: *first.File.ProjectID}
	task := f.f.createTask(p, "核对任务讨论与固定依据", false)
	values := map[string]any{"task_mentions_fixture": true, "project_id": p.ID, "task_id": task.ID, "alice_id": f.f.alice, "bob_id": f.f.bob, "file_id": first.File.ID, "file_version_id": first.Version.ID, "file_sha256": first.Version.SHA256, "current_version_id": second.Version.ID}
	if os.Getenv("CP_BROWSER_TASK_MENTIONS") == "1" {
		if _, err := f.f.owner.Exec(context.Background(), "INSERT INTO role_bindings(id,org_id,user_id,role_id,scope_type,classification_ceiling) SELECT $1,$2,$3,id,'org','project_restricted' FROM roles WHERE org_id=$2 AND key='contract_operator'", core.NewID(), f.f.org, f.f.alice); err != nil {
			t.Fatal(err)
		}
		f.f.aliceToken = memberMFAToken(f.f, "alice", "android")
		holdBrowserBusinessFixture(t, f.f, values)
	}
}

func drainTaskMentionFixture(ctx context.Context, f *fixture) error {
	queue := jobqueue.Queue{DB: f.api.DB}
	service := notification.Service{DB: f.api.DB}
	for i := 0; i < 1000; i++ {
		event, err := queue.DispatchOne(ctx, notification.Consumer())
		if err != nil {
			return err
		}
		if event == nil {
			break
		}
		if i == 999 {
			return fmt.Errorf("fixture dispatch exceeded bound")
		}
	}
	for i := 0; i < 1000; i++ {
		job, err := queue.Claim(ctx, []string{notification.JobKind}, "notification", "task-mentions.browser", time.Minute)
		if err != nil {
			return err
		}
		if job == nil {
			return nil
		}
		if _, err = service.Process(ctx, *job); err != nil {
			return err
		}
	}
	return fmt.Errorf("fixture delivery exceeded bound")
}

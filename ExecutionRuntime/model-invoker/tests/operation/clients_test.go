package operation_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/job"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/operation"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/operation/nativehttp"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/resource"
)

func TestResourceAndJobClientsRouteTypedLifecycleOperations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/files/file-1":
			_, _ = io.WriteString(w, `{"id":"file-1"}`)
		case "/files/file-1/content":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = io.WriteString(w, "file-bytes")
		case "/videos/job-1":
			_, _ = io.WriteString(w, `{"id":"job-1","status":"completed"}`)
		case "/videos/job-1/content":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = io.WriteString(w, "video-bytes")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	p, err := nativehttp.New(nativehttp.Config{
		Provider: "p", BaseURL: server.URL, Trust: nativehttp.TrustLocal, Auth: nativehttp.AuthAnonymous,
		Specs: []nativehttp.Spec{
			{Kind: operation.FileGet, Method: http.MethodGet, Path: "/files/{id}", RequiresResourceID: true, Lifecycle: operation.LifecycleResource, Support: operation.SupportNative, ResponseMode: nativehttp.ResponseJSON, IDKeys: []string{"id"}},
			{Kind: operation.FileContent, Method: http.MethodGet, Path: "/files/{id}/content", RequiresResourceID: true, Lifecycle: operation.LifecycleResource, Support: operation.SupportNative, ResponseMode: nativehttp.ResponseBinary, ArtifactKind: operation.ArtifactFile},
			{Kind: operation.VideoGet, Method: http.MethodGet, Path: "/videos/{id}", RequiresResourceID: true, Lifecycle: operation.LifecycleJob, Support: operation.SupportNative, ResponseMode: nativehttp.ResponseJSON, IDKeys: []string{"id"}, StatusKeys: []string{"status"}},
			{Kind: operation.VideoContent, Method: http.MethodGet, Path: "/videos/{id}/content", RequiresResourceID: true, Lifecycle: operation.LifecycleRequest, Support: operation.SupportNative, ResponseMode: nativehttp.ResponseBinary, ArtifactKind: operation.ArtifactVideo},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, _ := operation.NewRegistry(p)
	invoker, _ := operation.NewInvoker(registry)
	resources, _ := resource.NewClient(invoker)
	jobs, _ := job.NewClient(invoker)

	file, err := resources.Get(context.Background(), resource.Request{Provider: "p", Kind: resource.File, ID: "file-1"})
	if err != nil || file.Resource == nil || file.Resource.ID != "file-1" {
		t.Fatalf("file get = %+v, %v", file, err)
	}
	content, err := resources.Content(context.Background(), resource.Request{Provider: "p", Kind: resource.File, ID: "file-1", Budget: operation.Budget{MaxResponseBytes: 32}})
	if err != nil || len(content.Artifacts) != 1 || string(content.Artifacts[0].Data) != "file-bytes" {
		t.Fatalf("file content = %+v, %v", content, err)
	}
	video, err := jobs.Get(context.Background(), job.Request{Provider: "p", Kind: job.Video, ID: "job-1"})
	if err != nil || video.Job == nil || video.Job.Status != operation.StatusSucceeded {
		t.Fatalf("video get = %+v, %v", video, err)
	}
	videoContent, err := jobs.Results(context.Background(), job.Request{Provider: "p", Kind: job.Video, ID: "job-1"})
	if err != nil || len(videoContent.Artifacts) != 1 || string(videoContent.Artifacts[0].Data) != "video-bytes" {
		t.Fatalf("video content = %+v, %v", videoContent, err)
	}
	if _, err := jobs.Cancel(context.Background(), job.Request{Provider: modelinvoker.ProviderID("p"), Kind: job.Video, ID: "job-1"}); err == nil {
		t.Fatal("video cancel must not be confused with video delete")
	}
}

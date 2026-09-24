package webdav

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These cases mirror the parts of the litmus suite that a client actually
// trips over (MKCOL/PUT status codes, Overwrite handling, UNLOCK, Depth
// semantics) so they can regress locally without Docker.
func TestWebDAVConformanceStatusCodes(t *testing.T) {
	service, _, _ := newTestService(t)
	server := httptest.NewServer(service.Handler())
	t.Cleanup(server.Close)
	client := davClient(t, "admin")
	base := server.URL + "/dav/"

	if resp := do(t, client, "MKCOL", base+"dir", "", nil); resp.StatusCode != 201 {
		t.Fatalf("MKCOL status %d, want 201", resp.StatusCode)
	}
	if resp := do(t, client, "MKCOL", base+"dir", "", nil); resp.StatusCode != 405 {
		t.Fatalf("MKCOL on existing collection status %d, want 405", resp.StatusCode)
	}
	if resp := do(t, client, "MKCOL", base+"missing/child", "", nil); resp.StatusCode != 409 {
		t.Fatalf("MKCOL without parent status %d, want 409", resp.StatusCode)
	}
	if resp := do(t, client, "PUT", base+"dir/file.txt", "0123456789", nil); resp.StatusCode != 201 {
		t.Fatalf("PUT status %d, want 201", resp.StatusCode)
	}
	if resp := do(t, client, "PUT", base+"missing/file.txt", "x", nil); resp.StatusCode != 409 {
		t.Fatalf("PUT without parent status %d, want 409", resp.StatusCode)
	}
	if resp := do(t, client, "PUT", base+"dir", "x", nil); resp.StatusCode != 405 && resp.StatusCode != 409 {
		t.Fatalf("PUT over a collection status %d, want 405 or 409", resp.StatusCode)
	}
	if resp := do(t, client, "GET", base+"dir/", "", nil); resp.StatusCode != 405 {
		t.Fatalf("GET on a collection status %d, want 405", resp.StatusCode)
	}

	head := do(t, client, "HEAD", base+"dir/file.txt", "", nil)
	if head.StatusCode != 200 {
		t.Fatalf("HEAD status %d, want 200", head.StatusCode)
	}
	if length := head.Header.Get("Content-Length"); length != "10" {
		t.Fatalf("HEAD Content-Length %q, want 10", length)
	}
	if body, _ := io.ReadAll(head.Body); len(body) != 0 {
		t.Fatalf("HEAD returned a body of %d bytes", len(body))
	}

	depth0 := do(t, client, "PROPFIND", base+"dir/file.txt", "", map[string]string{"Depth": "0"})
	if depth0.StatusCode != 207 {
		t.Fatalf("PROPFIND Depth:0 status %d, want 207", depth0.StatusCode)
	}
	body, _ := io.ReadAll(depth0.Body)
	if count := strings.Count(string(body), "<D:response>"); count != 1 {
		t.Fatalf("PROPFIND Depth:0 returned %d responses, want 1: %s", count, body)
	}
	if strings.Contains(string(body), "/dav/dir/file.txt/") {
		t.Fatalf("file href must not end with a slash: %s", body)
	}
}

func TestWebDAVConformanceOverwriteAndUnlock(t *testing.T) {
	service, _, _ := newTestService(t)
	server := httptest.NewServer(service.Handler())
	t.Cleanup(server.Close)
	client := davClient(t, "admin")
	base := server.URL + "/dav/"

	if resp := do(t, client, "PUT", base+"a.txt", "source", nil); resp.StatusCode != 201 {
		t.Fatalf("PUT a.txt status %d", resp.StatusCode)
	}
	if resp := do(t, client, "PUT", base+"b.txt", "target", nil); resp.StatusCode != 201 {
		t.Fatalf("PUT b.txt status %d", resp.StatusCode)
	}

	// COPY refuses to clobber with Overwrite: F and leaves the target alone.
	copyRequest, _ := http.NewRequest("COPY", base+"a.txt", nil)
	copyRequest.Header.Set("Destination", base+"b.txt")
	copyRequest.Header.Set("Overwrite", "F")
	resp, err := client.Do(copyRequest)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 412 {
		t.Fatalf("COPY Overwrite:F status %d, want 412", resp.StatusCode)
	}
	if resp := do(t, client, "GET", base+"b.txt", "", nil); resp.StatusCode != 200 {
		t.Fatalf("target disappeared: status %d", resp.StatusCode)
	} else if body, _ := io.ReadAll(resp.Body); string(body) != "target" {
		t.Fatalf("COPY Overwrite:F modified the target: %q", body)
	}

	// With Overwrite: T the copy replaces the target.
	copyRequest, _ = http.NewRequest("COPY", base+"a.txt", nil)
	copyRequest.Header.Set("Destination", base+"b.txt")
	copyRequest.Header.Set("Overwrite", "T")
	resp, err = client.Do(copyRequest)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("COPY Overwrite:T status %d, want 204", resp.StatusCode)
	}
	if resp := do(t, client, "GET", base+"b.txt", "", nil); resp.StatusCode != 200 {
		t.Fatalf("copied file missing: status %d", resp.StatusCode)
	} else if body, _ := io.ReadAll(resp.Body); string(body) != "source" {
		t.Fatalf("copy content %q, want source", body)
	}

	// MOVE follows the same Overwrite rules.
	moveRequest, _ := http.NewRequest("MOVE", base+"a.txt", nil)
	moveRequest.Header.Set("Destination", base+"b.txt")
	moveRequest.Header.Set("Overwrite", "F")
	resp, err = client.Do(moveRequest)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 412 {
		t.Fatalf("MOVE Overwrite:F status %d, want 412", resp.StatusCode)
	}
	moveRequest, _ = http.NewRequest("MOVE", base+"a.txt", nil)
	moveRequest.Header.Set("Destination", base+"b.txt")
	moveRequest.Header.Set("Overwrite", "T")
	resp, err = client.Do(moveRequest)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("MOVE Overwrite:T status %d, want 204", resp.StatusCode)
	}
	if resp := do(t, client, "GET", base+"a.txt", "", nil); resp.StatusCode != 404 {
		t.Fatalf("source survived the move: status %d", resp.StatusCode)
	}

	// Unlocking with a token that was never issued is a conflict, not a crash.
	unlockRequest, _ := http.NewRequest("UNLOCK", base+"b.txt", nil)
	unlockRequest.Header.Set("Lock-Token", "<urn:uuid:00000000-0000-0000-0000-000000000000>")
	resp, err = client.Do(unlockRequest)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 409 {
		t.Fatalf("UNLOCK with unknown token status %d, want 409", resp.StatusCode)
	}
}

func TestWebDAVConformanceDepthAndDelete(t *testing.T) {
	service, storage, _ := newTestService(t)
	server := httptest.NewServer(service.Handler())
	t.Cleanup(server.Close)
	client := davClient(t, "admin")
	base := server.URL + "/dav/"

	if resp := do(t, client, "MKCOL", base+"tree", "", nil); resp.StatusCode != 201 {
		t.Fatalf("MKCOL tree status %d", resp.StatusCode)
	}
	if resp := do(t, client, "PUT", base+"tree/one.txt", "1", nil); resp.StatusCode != 201 {
		t.Fatalf("PUT one.txt status %d", resp.StatusCode)
	}
	if resp := do(t, client, "MKCOL", base+"tree/sub", "", nil); resp.StatusCode != 201 {
		t.Fatalf("MKCOL sub status %d", resp.StatusCode)
	}
	if resp := do(t, client, "PUT", base+"tree/sub/two.txt", "2", nil); resp.StatusCode != 201 {
		t.Fatalf("PUT two.txt status %d", resp.StatusCode)
	}

	depth1 := do(t, client, "PROPFIND", base, "", map[string]string{"Depth": "1"})
	if depth1.StatusCode != 207 {
		t.Fatalf("PROPFIND Depth:1 status %d", depth1.StatusCode)
	}
	body, _ := io.ReadAll(depth1.Body)
	if !strings.Contains(string(body), "tree") {
		t.Fatalf("Depth:1 listing is missing tree: %s", body)
	}
	if strings.Contains(string(body), "one.txt") || strings.Contains(string(body), "two.txt") {
		t.Fatalf("Depth:1 listing leaked grandchildren: %s", body)
	}

	// DELETE on a collection removes the whole subtree.
	if resp := do(t, client, "DELETE", base+"tree", "", nil); resp.StatusCode != 204 {
		t.Fatalf("DELETE collection status %d, want 204", resp.StatusCode)
	}
	if resp := do(t, client, "GET", base+"tree/one.txt", "", nil); resp.StatusCode != 404 {
		t.Fatalf("child survived the collection delete: status %d", resp.StatusCode)
	}
	if resp := do(t, client, "DELETE", base+"tree", "", nil); resp.StatusCode != 404 {
		t.Fatalf("second DELETE status %d, want 404", resp.StatusCode)
	}
	if _, err := storageFile(storage, "admin/tree/sub/two.txt"); err == nil {
		t.Fatal("subtree still exists on disk")
	}
}

func storageFile(storage, name string) ([]byte, error) {
	return os.ReadFile(filepath.Join(storage, filepath.FromSlash(name)))
}

// litmus parity for the lock behaviour the specification does promise: a
// conditional write with an unknown token must be rejected, and the lock owner
// may still PROPPATCH the resource (207 with a 403 propstat for the dead
// property we deliberately do not store).
func TestWebDAVConformanceLockConditions(t *testing.T) {
	service, _, _ := newTestService(t)
	server := httptest.NewServer(service.Handler())
	t.Cleanup(server.Close)
	client := davClient(t, "admin")
	base := server.URL + "/dav/"

	if resp := do(t, client, "PUT", base+"guarded.txt", "v1", nil); resp.StatusCode != 201 {
		t.Fatalf("seed PUT status %d", resp.StatusCode)
	}
	lockBody := `<?xml version="1.0"?><D:lockinfo xmlns:D="DAV:"><D:lockscope><D:exclusive/></D:lockscope><D:locktype><D:write/></D:locktype><D:owner>tester</D:owner></D:lockinfo>`
	resp := do(t, client, "LOCK", base+"guarded.txt", lockBody, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("LOCK status %d", resp.StatusCode)
	}
	token := resp.Header.Get("Lock-Token")
	if token == "" {
		t.Fatal("LOCK returned no token")
	}

	bogus := "<urn:uuid:00000000-0000-0000-0000-000000000000>"
	request, _ := http.NewRequest("PUT", base+"guarded.txt", strings.NewReader("v2"))
	request.Header.Set("If", fmt.Sprintf("(<%s>)", strings.Trim(bogus, "<>")))
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != 412 {
		t.Fatalf("conditional PUT with an unknown token status %d, want 412", response.StatusCode)
	}
	if resp := do(t, client, "GET", base+"guarded.txt", "", nil); resp.StatusCode != 200 {
		t.Fatalf("GET status %d", resp.StatusCode)
	} else if body, _ := io.ReadAll(resp.Body); string(body) != "v1" {
		t.Fatalf("rejected conditional PUT still wrote: %q", body)
	}

	// The owner presents the real token and may patch the locked resource.
	patchRequest, _ := http.NewRequest("PROPPATCH", base+"guarded.txt", strings.NewReader(
		`<?xml version="1.0"?><D:propertyupdate xmlns:D="DAV:"><D:set><D:prop><litmus:random xmlns:litmus="http://webdav.org/neon/litmus/">foobar</litmus:random></D:prop></D:set></D:propertyupdate>`))
	patchRequest.Header.Set("Content-Type", "application/xml")
	patchRequest.Header.Set("If", fmt.Sprintf("(<%s>)", strings.Trim(token, "<>")))
	response, err = client.Do(patchRequest)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != 207 {
		t.Fatalf("PROPPATCH by the lock owner status %d, want 207: %s", response.StatusCode, body)
	}
	if !strings.Contains(string(body), "403") {
		t.Fatalf("expected a 403 propstat for the unsupported property: %s", body)
	}
}

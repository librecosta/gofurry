package collection

import (
	json "encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"uuid"

	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

func TestManagedWorkerNodeProxy(t *testing.T) {
	actor := dbgen.RecordManagementOperationParams{
		RequestID: uuid.NewV7(), ActorID: uuid.NewV7(), ActorName: "已认证运营员", ActorRole: "operator",
	}
	calls := make(chan string, 4)
	harvest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("namespace") != "gofurry-cn" {
			t.Errorf("foreign namespace: %q", r.URL.RawQuery)
		}
		calls <- r.Header.Get("Authorization")
		switch r.URL.Path {
		case "/api/v1/operations/worker-nodes":
			if r.URL.Query().Get("after") != "node-00" {
				t.Errorf("invalid pagination: %q", r.URL.RawQuery)
			}
			io.WriteString(w, `{"items":[{"node_id":"node-01","can_manage":true,"proxy_url":"https://private.invalid/"}]}`)
		case "/api/v1/operations/worker-nodes/node-01":
			io.WriteString(w, `{"node_id":"node-01","can_manage":true}`)
		case "/api/v1/operations/worker-nodes/node-01/state":
			var received struct {
				WorkerNodeEdit
				RequestID uuid.UUID `json:"request_id"`
				ActorID   uuid.UUID `json:"actor_id"`
				ActorName string    `json:"actor_name"`
				ActorRole string    `json:"actor_role"`
			}
			raw, err := io.ReadAll(r.Body)
			if err != nil || json.Unmarshal(raw, &received, json.RejectUnknownMembers(true)) != nil ||
				received.ActorID != actor.ActorID || received.RequestID != actor.RequestID || received.ActorName != actor.ActorName ||
				received.ActorRole != actor.ActorRole || received.Revision != 3 || received.State != "paused" {
				t.Errorf("remote request did not carry the authenticated actor and revision")
			}
			io.WriteString(w, `{"revision":4}`)
		default:
			t.Errorf("unexpected Harvest path %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer harvest.Close()
	service := &Service{namespace: "gofurry-cn", remote: &client{base: harvest.URL, reader: "reader", operator: "operator", http: harvest.Client()}}
	list, err := service.ManagedWorkerNodes(t.Context(), false, "node-00")
	if err != nil || len(list.Items) != 1 || list.Items[0].CanManage || <-calls != "Bearer reader" {
		t.Fatalf("invalid read-only result: %+v (%v)", list, err)
	}
	data, err := json.Marshal(list)
	if err != nil || strings.Contains(string(data), "private.invalid") {
		t.Fatal("unexpected remote URL leaked")
	}
	node, err := service.ManagedWorkerNode(t.Context(), true, "node-01")
	if err != nil || node.NodeID != "node-01" || !node.CanManage || <-calls != "Bearer operator" {
		t.Fatalf("operator detail: %+v (%v)", node, err)
	}
	if _, err := service.ManagedWorkerNode(t.Context(), true, "../global"); err != ErrIntent {
		t.Fatal("unsafe node ID was accepted")
	}
	untrusted := actor
	untrusted.ActorRole = "developer"
	edit := WorkerNodeEdit{Revision: 3, State: "paused"}
	if _, err := service.ChangeWorkerNode(t.Context(), "node-01", edit, untrusted); err != ErrCollectionPermission {
		t.Fatal("developer mutation was accepted")
	}
	rev, err := service.ChangeWorkerNode(t.Context(), "node-01", edit, actor)
	if err != nil || rev != 4 || <-calls != "Bearer operator" {
		t.Fatalf("control failed: revision %d, %v", rev, err)
	}
	if len(calls) != 0 {
		t.Fatal("unexpected remote calls")
	}
}

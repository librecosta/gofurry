package collection

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"time"
	"uuid"

	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

var workerNodeID = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

const workerNodesPath = "operations/worker-nodes"

type WorkerNode struct {
	NodeID          string       `json:"node_id"`
	RuntimeID       uuid.UUID    `json:"runtime_id"`
	DesiredState    string       `json:"desired_state"`
	Revision        int64        `json:"revision"`
	Online          bool         `json:"online"`
	RuntimeDraining bool         `json:"runtime_draining"`
	LastSeen        time.Time    `json:"last_seen"`
	Capacity        int32        `json:"capacity"`
	ActiveTasks     int64        `json:"active_tasks"`
	Capabilities    []string     `json:"capabilities"`
	EgressProfiles  []string     `json:"egress_profiles"`
	Paths           []WorkerPath `json:"paths"`
	CanManage       bool         `json:"can_manage"`
	Tasks           []WorkerTask `json:"tasks,omitempty"`
}

type WorkerPath struct {
	Profile  string `json:"profile"`
	Mode     string `json:"mode"`
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Region   string `json:"region"`
	State    string `json:"state"`
	Capacity int32  `json:"capacity"`
}

type WorkerTask struct {
	JobID      uuid.UUID `json:"job_id"`
	Capability string    `json:"capability"`
	ClaimedAt  time.Time `json:"claimed_at"`
}

type WorkerNodeList struct {
	Items []WorkerNode `json:"items"`
	Next  string       `json:"next,omitempty"`
}

type WorkerNodeEdit struct {
	Revision int64  `json:"revision"`
	State    string `json:"state"`
}

func workerNodeError(err error) error {
	if errors.Is(err, remoteStatus(http.StatusBadRequest)) {
		return ErrIntent
	}
	return collectionControlError(err)
}

func (s *Service) readWorkerNodes(ctx context.Context, operator bool, path string, values url.Values, output any) error {
	if s == nil || s.remote == nil || s.remote.reader == "" || s.namespace == "" {
		return errNetwork
	}
	values.Set("namespace", s.namespace)
	token := s.remote.reader
	if operator && s.remote.operator != "" {
		token = s.remote.operator
	}
	return workerNodeError(s.remote.call(ctx, token, http.MethodGet, path, values, nil, output, http.StatusOK))
}

func (s *Service) ManagedWorkerNodes(ctx context.Context, operator bool, cursor string) (WorkerNodeList, error) {
	out := WorkerNodeList{Items: []WorkerNode{}}
	values := url.Values{}
	if cursor != "" {
		if !workerNodeID.MatchString(cursor) {
			return out, ErrIntent
		}
		values.Set("after", cursor)
	}
	err := s.readWorkerNodes(ctx, operator, workerNodesPath, values, &out)
	if err != nil {
		return WorkerNodeList{}, err
	}
	for i := range out.Items {
		out.Items[i].CanManage = out.Items[i].CanManage && operator && s.remote.operator != ""
	}
	return out, nil
}

func (s *Service) ManagedWorkerNode(ctx context.Context, operator bool, id string) (WorkerNode, error) {
	var out WorkerNode
	if !workerNodeID.MatchString(id) {
		return out, ErrIntent
	}
	err := s.readWorkerNodes(ctx, operator, workerNodesPath+"/"+id, url.Values{}, &out)
	if err == nil {
		out.CanManage = out.CanManage && operator && s.remote.operator != ""
	}
	return out, err
}

func (s *Service) ChangeWorkerNode(ctx context.Context, id string, in WorkerNodeEdit, actor dbgen.RecordManagementOperationParams) (int64, error) {
	if !workerNodeID.MatchString(id) || in.Revision < 1 || (in.State != "active" && in.State != "paused" && in.State != "draining") {
		return 0, ErrIntent
	}
	if s == nil || s.remote == nil || s.namespace == "" || s.remote.operator == "" ||
		actor.RequestID == uuid.Nil() || actor.ActorID == uuid.Nil() || (actor.ActorRole != "owner" && actor.ActorRole != "operator") {
		return 0, ErrCollectionPermission
	}
	input := struct {
		WorkerNodeEdit
		RequestID uuid.UUID `json:"request_id"`
		ActorID   uuid.UUID `json:"actor_id"`
		ActorName string    `json:"actor_name"`
		ActorRole string    `json:"actor_role"`
	}{in, actor.RequestID, actor.ActorID, actor.ActorName, actor.ActorRole}
	var result struct {
		Revision int64 `json:"revision"`
	}
	err := s.remote.call(ctx, s.remote.operator, http.MethodPost, workerNodesPath+"/"+id+"/state",
		url.Values{"namespace": {s.namespace}}, input, &result, http.StatusOK)
	if err == nil && result.Revision != in.Revision+1 {
		err = errProtocol
	}
	return result.Revision, workerNodeError(err)
}

package collection

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"
	"uuid"

	"github.com/librecosta/gofurry/services/api/internal/database/dbgen"
)

const egressPolicyPath = "operations/egress-policy"

var (
	ErrEgressPolicyInvalid  = errors.New("所选能力、线路配置、模式或出口地区不符合 Harvest 策略")
	ErrEgressPolicyConflict = errors.New("出网策略版本已变化，请查看远端版本后再提交")
)

type WorkerHealth struct {
	WindowHours int           `json:"window_hours"`
	Checks      []HealthCheck `json:"checks"`
}

type HealthCheck struct {
	Profile             string    `json:"profile"`
	PathID              string    `json:"path_id"`
	Kind                string    `json:"kind"`
	Region              string    `json:"region"`
	CheckType           string    `json:"check_type"`
	LastOutcome         string    `json:"last_outcome"`
	LastAt              time.Time `json:"last_at"`
	LastLatencyMS       int32     `json:"last_latency_ms"`
	ConsecutiveFailures int64     `json:"consecutive_failures"`
	Successes           int64     `json:"successes"`
	Failures            int64     `json:"failures"`
	TransportFailures   int64     `json:"transport_failures"`
	LatencyMS           int64     `json:"latency_ms"`
	Recoveries          int64     `json:"recoveries"`
}

type EgressPolicyView struct {
	EgressPolicyStatus
	CanEdit   bool               `json:"can_edit"`
	Truncated bool               `json:"truncated"`
	Rules     []EgressPolicyRule `json:"rules"`
}

type EgressPolicyStatus struct {
	Status          string `json:"status"`
	Revision        int64  `json:"revision"`
	RoutingRevision int64  `json:"routing_revision"`
}

type EgressPolicyRule struct {
	Capability string          `json:"capability"`
	Profile    string          `json:"profile"`
	Mode       string          `json:"mode"`
	Region     string          `json:"region"`
	Reason     string          `json:"reason"`
	Options    []EgressProfile `json:"options"`
	Workers    []EgressMatch   `json:"workers"`
}

type EgressProfile struct {
	Profile     string   `json:"profile"`
	Mode        string   `json:"mode"`
	Regions     []string `json:"regions"`
	AllowDirect bool     `json:"allow_direct"`
}

type EgressMatch struct {
	NodeID    string       `json:"node_id,omitempty"`
	RuntimeID *uuid.UUID   `json:"runtime_id,omitempty"`
	Shared    bool         `json:"shared"`
	Reason    string       `json:"reason"`
	Paths     []WorkerPath `json:"paths"`
}

type EgressPolicyEdit struct {
	ExpectedRevision int64  `json:"expected_revision"`
	Capability       string `json:"capability"`
	Profile          string `json:"profile"`
	Mode             string `json:"mode"`
	Region           string `json:"region"`
}

func (s *Service) ManagedWorkerHealth(ctx context.Context, node string) (WorkerHealth, error) {
	var out WorkerHealth
	if !workerNodeID.MatchString(node) {
		return out, ErrIntent
	}
	err := s.readWorkerNodes(ctx, false, workerNodesPath+"/"+node+"/egress-health", url.Values{}, &out)
	return out, err
}

func (s *Service) ManagedEgressPolicy(ctx context.Context, operator bool) (EgressPolicyView, error) {
	var out EgressPolicyView
	err := s.readWorkerNodes(ctx, operator, egressPolicyPath, url.Values{}, &out)
	if err == nil {
		out.CanEdit = out.CanEdit && operator && s.remote.operator != ""
	}
	return out, err
}

func (s *Service) ChangeEgressPolicy(ctx context.Context, in EgressPolicyEdit, actor dbgen.RecordManagementOperationParams) (int64, error) {
	if s == nil || s.remote == nil || s.namespace == "" || s.remote.operator == "" ||
		actor.RequestID == uuid.Nil() || actor.ActorID == uuid.Nil() || (actor.ActorRole != "owner" && actor.ActorRole != "operator") {
		return 0, ErrCollectionPermission
	}
	payload := struct {
		EgressPolicyEdit
		RequestID uuid.UUID `json:"request_id"`
		ActorID   uuid.UUID `json:"actor_id"`
		ActorName string    `json:"actor_name"`
		ActorRole string    `json:"actor_role"`
	}{in, actor.RequestID, actor.ActorID, actor.ActorName, actor.ActorRole}
	var out EgressPolicyStatus
	err := s.remote.call(ctx, s.remote.operator, http.MethodPatch, egressPolicyPath,
		url.Values{"namespace": {s.namespace}}, payload, &out, http.StatusOK)
	switch {
	case errors.Is(err, remoteStatus(http.StatusBadRequest)):
		return 0, ErrEgressPolicyInvalid
	case errors.Is(err, remoteStatus(http.StatusConflict)):
		return 0, ErrEgressPolicyConflict
	case err != nil:
		return 0, collectionControlError(err)
	case out.Revision != in.ExpectedRevision+1:
		return 0, errProtocol
	default:
		return out.Revision, nil
	}
}

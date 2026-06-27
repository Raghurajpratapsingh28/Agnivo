package worker_test

import (
	"encoding/json"
	"testing"

	deploycancel "github.com/agnivo/agnivo/packages/application/deploy/cancel"
	"github.com/agnivo/agnivo/packages/application/deploy/worker"
	"github.com/agnivo/agnivo/packages/application/controlplane/cpjobs"
	"github.com/agnivo/agnivo/packages/platform/jobs"
	"github.com/stretchr/testify/assert"
)

func TestHandleInvalidPayload(t *testing.T) {
	h := worker.NewHandler(nil, deploycancel.NewRegistry())
	err := h.Handle(t.Context(), jobs.Job{Payload: json.RawMessage(`not-json`)})
	assert.Error(t, err)
}

func TestCancelDeployment(t *testing.T) {
	cancels := deploycancel.NewRegistry()
	h := worker.NewHandler(nil, cancels)
	assert.False(t, h.CancelDeployment("missing"))
}

func TestPayloadDecode(t *testing.T) {
	payload := cpjobs.Payload{OrgID: "o1", ProjectID: "p1", DeploymentID: "d1"}
	raw, err := json.Marshal(payload)
	assert.NoError(t, err)
	var decoded cpjobs.Payload
	assert.NoError(t, json.Unmarshal(raw, &decoded))
	assert.Equal(t, "d1", decoded.DeploymentID)
}

func TestUnknownJobType(t *testing.T) {
	h := worker.NewHandler(nil, deploycancel.NewRegistry())
	payload, _ := json.Marshal(cpjobs.Payload{DeploymentID: "d1"})
	err := h.Handle(t.Context(), jobs.Job{Type: "unknown.type", Payload: payload})
	assert.Error(t, err)
}

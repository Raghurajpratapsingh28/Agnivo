package worker_test

import (
	"encoding/json"
	"testing"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/cancel"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/worker"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/controlplane/cpjobs"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/jobs"
	"github.com/stretchr/testify/assert"
)

func TestHandleInvalidPayload(t *testing.T) {
	h := worker.NewHandler(nil, cancel.NewRegistry())
	err := h.Handle(t.Context(), jobs.Job{Payload: json.RawMessage(`not-json`)})
	assert.Error(t, err)
}

func TestCancelBuild(t *testing.T) {
	cancels := cancel.NewRegistry()
	h := worker.NewHandler(nil, cancels)
	assert.False(t, h.CancelBuild("missing"))
}

func TestPayloadDecode(t *testing.T) {
	payload := cpjobs.Payload{OrgID: "o1", ProjectID: "p1", DeploymentID: "d1"}
	raw, err := json.Marshal(payload)
	assert.NoError(t, err)
	var decoded cpjobs.Payload
	assert.NoError(t, json.Unmarshal(raw, &decoded))
	assert.Equal(t, "d1", decoded.DeploymentID)
}

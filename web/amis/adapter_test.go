package amis

import (
	"strings"
	"testing"
)

func TestDefaultAdapterToJS(t *testing.T) {
	a := DefaultResponseAdapter()
	js := string(a.ToJS())

	// Must contain field mappings
	if !strings.Contains(js, `payload["code"]`) {
		t.Error("missing code field")
	}
	if !strings.Contains(js, `payload["message"]`) {
		t.Error("missing message field")
	}
	if !strings.Contains(js, `payload["data"]`) {
		t.Error("missing data field")
	}
	if !strings.Contains(js, `"000000"`) {
		t.Error("missing success value")
	}

	// Must NOT contain HumaResponse or framework-specific logic
	for _, banned := range []string{"HumaResponse", "payload.body", "body.code"} {
		if strings.Contains(js, banned) {
			t.Errorf("JS contains banned pattern: %s", banned)
		}
	}

	t.Logf("Generated JS:\n%s", js)
}

func TestCustomAdapterToJS(t *testing.T) {
	a := ResponseAdapter{
		CodeField:    "errno",
		MessageField: "errmsg",
		DataField:    "result",
		SuccessValue: "0",
	}
	js := string(a.ToJS())

	if !strings.Contains(js, `payload["errno"]`) {
		t.Error("missing errno field")
	}
	if !strings.Contains(js, `payload["errmsg"]`) {
		t.Error("missing errmsg field")
	}
	if !strings.Contains(js, `payload["result"]`) {
		t.Error("missing result field")
	}
	if !strings.Contains(js, `"0"`) {
		t.Error("missing success value 0")
	}

	t.Logf("Custom JS:\n%s", js)
}

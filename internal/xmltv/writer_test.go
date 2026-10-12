package xmltv

import (
	"bytes"
	"strings"
	"testing"

	"github.com/patrickjmcd/hdhr2xmltv/internal/hdhr"
)

func TestWriteIncludesAffiliateDisplayName(t *testing.T) {
	var buf bytes.Buffer
	err := NewWriter(&buf).Write([]hdhr.GuideChannel{
		{GuideNumber: "5.1", GuideName: "KCTVDT", Affiliate: "CBS"},
		{GuideNumber: "19.2", GuideName: "PBS", Affiliate: "PBS"},
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "<display-name>CBS</display-name>") {
		t.Errorf("affiliate missing from output:\n%s", out)
	}
	if strings.Count(out, "<display-name>PBS</display-name>") != 1 {
		t.Errorf("affiliate equal to the guide name should not be repeated:\n%s", out)
	}
}

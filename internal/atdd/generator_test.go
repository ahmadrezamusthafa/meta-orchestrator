package atdd

import (
	"strings"
	"testing"
)

func TestATDDGeneratorPlaywrightSpec(t *testing.T) {
	generator := NewATDDGenerator()

	cases := []ATDDTestCase{
		{
			ScenarioName:  "User views checkout summary and submits order",
			TargetRoute:   "/checkout",
			ActionSteps:   []string{"Click #agree-terms checkbox", "Click #pay-button"},
			AssertElement: "#order-confirmation-badge",
			ExpectedText:  "Order Confirmed",
		},
	}

	spec := generator.GeneratePlaywrightSpec("User Checkout", cases, nil)

	if !strings.Contains(spec, "import { test, expect } from '@playwright/test';") {
		t.Errorf("missing Playwright import in spec")
	}
	if !strings.Contains(spec, "await page.goto('/checkout');") {
		t.Errorf("missing page.goto in spec")
	}
	if !strings.Contains(spec, "page.locator('#order-confirmation-badge')") {
		t.Errorf("missing locator assertion in spec")
	}
	if !strings.Contains(spec, "Order Confirmed") {
		t.Errorf("missing expected text in spec")
	}
}

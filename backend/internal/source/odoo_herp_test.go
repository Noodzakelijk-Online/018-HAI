package source

import (
	"automation-hub-backend/internal/models"
	"strings"
	"testing"
)

func TestOdooHERPItemsContainGovernedWorkflowSignals(t *testing.T) {
	source := &models.ConnectedSource{
		ConnectorKey:      "odoo-herp",
		Name:              "Odoo workspace",
		DefaultProjectKey: "Robert-life-os",
	}

	items := odooHERPItems(source, ImportRequest{})
	if len(items) < 15 {
		t.Fatalf("items = %d, want broad Odoo app coverage", len(items))
	}

	joined := strings.Join([]string{items[0].Content, items[0].Metadata, items[0].SourceURI}, "\n")
	for _, want := range []string{"Task:", "Follow up:", "Decision:", "read_only_default=true", "odoo://app/"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("Odoo HERP item missing %q:\n%s", want, joined)
		}
	}
}

func TestOdooHERPItemsCanBeFilteredBySelectedApps(t *testing.T) {
	source := &models.ConnectedSource{
		ConnectorKey:      "odoo-herp",
		Name:              "Odoo workspace",
		SyncTarget:        "https://example.odoo.com/odoo?apps=CRM,Sales",
		DefaultProjectKey: "Robert-life-os",
	}

	items := odooHERPItems(source, ImportRequest{})
	if len(items) != 2 {
		t.Fatalf("items = %d, want two selected app domains", len(items))
	}
	if items[0].ExternalID != "odoo-herp-app:crm" || items[1].ExternalID != "odoo-herp-app:sales" {
		t.Fatalf("items = %#v, want CRM and Sales in requested order", items)
	}
}

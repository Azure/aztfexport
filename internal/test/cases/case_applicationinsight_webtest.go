package cases

import (
	"fmt"

	"github.com/Azure/aztfy/internal/test"

	"github.com/Azure/aztfy/internal/resmap"
)

var _ Case = CaseApplicationInsightWebTest{}

type CaseApplicationInsightWebTest struct{}

func (CaseApplicationInsightWebTest) Tpl(d test.Data) string {
	return fmt.Sprintf(`
provider "azurerm" {
  features {
    resource_group {
      prevent_deletion_if_contains_resources = false
    }
  }
}
resource "azurerm_resource_group" "test" {
  name     = "%[1]s"
  location = "WestEurope"
}
resource "azurerm_application_insights" "test" {
  name                = "test-%[2]s"
  location            = azurerm_resource_group.test.location
  resource_group_name = azurerm_resource_group.test.name
  application_type    = "web"
}
resource "azurerm_application_insights_web_test" "test" {
  name                    = "test-%[2]s"
  location                = azurerm_resource_group.test.location
  resource_group_name     = azurerm_resource_group.test.name
  application_insights_id = azurerm_application_insights.test.id
  kind                    = "ping"
  geo_locations           = ["us-tx-sn1-azr"]
  configuration           = <<XML
<WebTest Name="WebTest1" Id="ABD48585-0831-40CB-9069-682EA6BB3583" Enabled="True" CssProjectStructure="" CssIteration="" Timeout="0" WorkItemIds="" xmlns="http://microsoft.com/schemas/VisualStudio/TeamTest/2010" Description="" CredentialUserName="" CredentialPassword="" PreAuthenticate="True" Proxy="default" StopOnError="False" RecordedResultFile="" ResultsLocale="">
  <Items>
    <Request Method="GET" Guid="a5f10126-e4cd-570d-961c-cea43999a200" Version="1.1" Url="http://microsoft.com" ThinkTime="0" Timeout="300" ParseDependentRequests="True" FollowRedirects="True" RecordResult="True" Cache="False" ResponseTimeGoal="0" Encoding="utf-8" ExpectedHttpStatusCode="200" ExpectedResponseUrl="" ReportingName="" IgnoreHttpStatusCode="False" />
  </Items>
</WebTest>
XML
  lifecycle {
    ignore_changes = ["tags"]
  }
}
`, d.RandomRgName(), d.RandomStringOfLength(8))
}

func (CaseApplicationInsightWebTest) ResourceMapping(d test.Data) (resmap.ResourceMapping, error) {
	return test.ResourceMapping(fmt.Sprintf(`{
{{ "/subscriptions/%[1]s/resourcegroups/%[2]s" | ToUpper | Quote }}: {
  "resource_type": "azurerm_resource_group",
  "resource_name": "test",
  "resource_id": "/subscriptions/%[1]s/resourceGroups/%[2]s"
},
{{ "/subscriptions/%[1]s/resourcegroups/%[2]s/providers/microsoft.insights/components/test-%[3]s" | ToUpper | Quote }}: {
  "resource_type": "azurerm_application_insights",
  "resource_name": "test",
  "resource_id": "/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.Insights/components/test-%[3]s"
},
{{ "/subscriptions/%[1]s/resourcegroups/%[2]s/providers/microsoft.insights/webtests/test-%[3]s" | ToUpper | Quote }}: {
  "resource_type": "azurerm_application_insights_web_test",
  "resource_name": "test",
  "resource_id": "/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.Insights/webTests/test-%[3]s"
}
}
`, d.SubscriptionId, d.RandomRgName(), d.RandomStringOfLength(8)))
}

func (CaseApplicationInsightWebTest) SingleResourceContext(d test.Data) ([]SingleResourceContext, error) {
	return []SingleResourceContext{
		{
			AzureId:             fmt.Sprintf("/subscriptions/%[1]s/resourceGroups/%[2]s", d.SubscriptionId, d.RandomRgName()),
			ExpectResourceCount: 1,
		},
		{
			AzureId:             fmt.Sprintf("/subscriptions/%[1]s/resourceGroups/%[2]s/providers/microsoft.insights/components/test-%[3]s", d.SubscriptionId, d.RandomRgName(), d.RandomStringOfLength(8)),
			ExpectResourceCount: 1,
		},
		{
			AzureId:             fmt.Sprintf("/subscriptions/%[1]s/resourceGroups/%[2]s/providers/Microsoft.insights/webTests/test-%[3]s", d.SubscriptionId, d.RandomRgName(), d.RandomStringOfLength(8)),
			ExpectResourceCount: 1,
		},
	}, nil
}

package scalingo

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/Scalingo/go-scalingo/v5"
)

func dataSourceScInvoice() *schema.Resource {
	return &schema.Resource{
		ReadContext: dataSourceScInvoiceRead,

		Schema: map[string]*schema.Schema{
			"before": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"after": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"invoices": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"id": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"total_price": {
							Type:     schema.TypeInt,
							Computed: true,
						},
						"total_price_with_vat": {
							Type:     schema.TypeInt,
							Computed: true,
						},
						"billing_month": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"pdf_url": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"invoice_number": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"state": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"vat_rate": {
							Type:     schema.TypeInt,
							Computed: true,
						},
						"items": {
							Type:     schema.TypeList,
							Computed: true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"id": {
										Type:     schema.TypeString,
										Computed: true,
									},
									"label": {
										Type:     schema.TypeString,
										Computed: true,
									},
									"price": {
										Type:     schema.TypeInt,
										Computed: true,
									},
								},
							},
						},
						"detailed_items": {
							Type:     schema.TypeList,
							Computed: true,
							Elem: &schema.Resource{
								Schema: map[string]*schema.Schema{
									"id": {
										Type:     schema.TypeString,
										Computed: true,
									},
									"label": {
										Type:     schema.TypeString,
										Computed: true,
									},
									"price": {
										Type:     schema.TypeInt,
										Computed: true,
									},
									"app": {
										Type:     schema.TypeString,
										Computed: true,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

const PageSize = 50

func isInTimeRange(min time.Time, max time.Time, value time.Time) bool {
	// If no boundary is set, the time is considered always in the range
	if min.IsZero() && max.IsZero() {
		return true
	} else if min.IsZero() {
		return value.Before(max)
	} else if max.IsZero() {
		return value.After(min)
	} else {
		return value.After(min) && value.Before(max)
	}
}

func structToMap(v interface{}) (map[string]interface{}, error) {
	var rawStruct map[string]interface{}
	jsonStruct, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(jsonStruct, &rawStruct)
	if err != nil {
		return nil, err
	}
	return rawStruct, err
}

func dataSourceScInvoiceRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var (
		// All invoices which concern a month after a given date
		afterTime time.Time
		// All invoices which concern a month before a given date
		beforeTime time.Time
		err        error
	)

	client, _ := meta.(*scalingo.Client)

	// Handling date filters
	beforeTimeStr, _ := d.Get("before").(string)
	if beforeTimeStr != "" {
		beforeTime, err = time.Parse(scalingo.BillingMonthDateFormat, beforeTimeStr)
		if err != nil {
			return diag.Errorf("fail to parse before: %v", err)
		}
	}

	afterTimeStr, _ := d.Get("after").(string)
	if afterTimeStr != "" {
		afterTime, err = time.Parse(scalingo.BillingMonthDateFormat, afterTimeStr)
		if err != nil {
			return diag.Errorf("fail to parse after: %v", err)
		}
	}

	// fetch all invoices in a slice
	maxPage := 1
	currentPage := 1
	var invoices []*scalingo.Invoice
	for currentPage <= maxPage {
		pageInvoices, pagination, err := client.InvoicesList(ctx, scalingo.PaginationOpts{
			Page:    currentPage,
			PerPage: PageSize,
		})
		if err != nil {
			return diag.Errorf("fail to list invoices: %v", err)
		}
		if currentPage == 1 {
			maxPage = pagination.TotalPages
			invoices = make([]*scalingo.Invoice, 0, pagination.TotalCount)
		}
		invoices = append(invoices, pageInvoices...)
		currentPage++
	}

	// filtering invoices with the current config
	filteredInvoices := keepIf(invoices, func(invoice *scalingo.Invoice) bool {
		return isInTimeRange(afterTime, beforeTime, time.Time(invoice.BillingMonth))
	})

	// mapping invoices list to raw json struct before saving in the state to keep json fields
	invoicesState, err := structToMap(map[string]interface{}{
		"before":   beforeTimeStr,
		"after":    afterTimeStr,
		"invoices": filteredInvoices,
	})

	for i, invoice := range invoicesState["invoices"].([]interface{}) {
		invoice.(map[string]interface{})["billing_month"] = time.Time(filteredInvoices[i].BillingMonth).Format(time.RFC3339)
	}
	if err != nil {
		return diag.Errorf("fail to map invoices: %v", err)
	}

	// saving in the state
	err = SetAll(d, invoicesState)
	if err != nil {
		return diag.Errorf("fail to store invoices information: %v", err)
	}

	// use period as an ID
	d.SetId(fmt.Sprintf(
		"%s-%s",
		beforeTime.Format(scalingo.BillingMonthDateFormat),
		afterTime.Format(scalingo.BillingMonthDateFormat),
	))

	return nil
}

package llm

// SystemPrompt returns the extraction system prompt for document processing.
func SystemPrompt() string {
	return `You are a strict JSON extraction engine. You output ONLY a valid JSON object. No thinking. No <thought> tags. No explanations. Just JSON.

Extract document details. Return a JSON object with these fields:
- classification: "invoice", "warranty", "amc", or "other"
- brand: string or null
- model: string or null
- serial_number: string or null
- purchase_date: ISO 8601 date string or null
- warranty_end: ISO 8601 date string or null
- price: string (exact decimal, e.g. "3999.99") or null
- currency: 3-letter code (e.g. "INR", "USD") or null
- metadata: object of additional useful fields found in the document (snake_case keys)

Metadata examples: invoice_number, amc_card_number, amc_type, amc_period, customer_name, service_branch, icr_number, tax_rate, po_reference, system_order_no.

Only include fields you can clearly read. Use null for absent fields. Dates in ISO 8601 (YYYY-MM-DD). Price as exact decimal string.`
}

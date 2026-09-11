import { http, HttpResponse } from 'msw';

const userId = 'alice';

const assets = [
  { id: 'a1', owner_household_id: null, created_at: '2026-08-01T00:00:00Z', updated_at: '2026-08-01T00:00:00Z', data: { name: null, brand: 'Samsung', model: 'Galaxy S23', serial_number: 'SN1001', asset_category: null, category_confidence: null, category_user_set: null, purchase_date: '2026-08-01T00:00:00Z', warranty_end: '2027-08-01T00:00:00Z', price: '75000.00', currency: 'INR', metadata: {}, confidence: null, deleted_at: null, merged_into: null, merged_at: null, merged_assets: null } },
  { id: 'a2', owner_household_id: null, created_at: '2026-08-01T00:00:00Z', updated_at: '2026-08-01T00:00:00Z', data: { name: null, brand: 'LG', model: 'Washing Machine', serial_number: 'SN2002', asset_category: null, category_confidence: null, category_user_set: null, purchase_date: '2024-01-01T00:00:00Z', warranty_end: '2025-01-01T00:00:00Z', price: '35000.00', currency: 'INR', metadata: {}, confidence: null, deleted_at: null, merged_into: null, merged_at: null, merged_assets: null } },
  { id: 'a3', owner_household_id: null, created_at: '2026-08-05T00:00:00Z', updated_at: '2026-08-05T00:00:00Z', data: { name: null, brand: 'Sony', model: 'Bravia TV', serial_number: 'SN3003', asset_category: null, category_confidence: null, category_user_set: null, purchase_date: '2025-06-01T00:00:00Z', warranty_end: '2026-06-01T00:00:00Z', price: '120000.00', currency: 'EUR', metadata: {}, confidence: null, deleted_at: null, merged_into: null, merged_at: null, merged_assets: null } },
  { id: 'a4', owner_household_id: null, created_at: '2026-08-10T00:00:00Z', updated_at: '2026-08-10T00:00:00Z', data: { name: null, brand: 'Bosch', model: 'Dishwasher', serial_number: 'SN4004', asset_category: null, category_confidence: null, category_user_set: null, purchase_date: '2026-08-10T00:00:00Z', warranty_end: '2028-08-10T00:00:00Z', price: '45000.00', currency: 'INR', metadata: {}, confidence: null, deleted_at: null, merged_into: null, merged_at: null, merged_assets: null } },
];

const documents = [
  ...assets.map((a, i) => ({ id: `d${i * 2 + 1}`, asset_id: a.id, source_id: `s${i * 2 + 1}`, source_filename: `doc_${a.id}_1.pdf`, source_uploaded_at: a.created_at, owner_household_id: null, status: 'processed', created_at: a.created_at, updated_at: a.created_at, data: { doc_type: 'invoice', confidence: 0.9, extracted_fields: null, raw_extraction: null, user_directive: null } })),
  ...assets.map((a, i) => ({ id: `d${i * 2 + 2}`, asset_id: a.id, source_id: `s${i * 2 + 2}`, source_filename: `doc_${a.id}_2.pdf`, source_uploaded_at: a.created_at, owner_household_id: null, status: 'processed', created_at: a.created_at, updated_at: a.created_at, data: { doc_type: 'other', confidence: 0.9, extracted_fields: null, raw_extraction: null, user_directive: null } })),
];

/**
 * The documents list (task 8.1) serves the same documents plus a top-level
 * derived `status` (server-side) and the asset link for processed rows. The
 * first two documents link to assets a1/a2; the rest are unlinked.
 */
const documentRows = [
  { id: 'd1', status: 'processed', asset_id: 'a1', source_id: 's1', source_filename: 'invoice_microwave.pdf', source_uploaded_at: '2026-08-01T10:00:00Z', owner_household_id: null, created_at: '2026-08-01T10:00:00Z', updated_at: '2026-08-01T10:00:00Z', data: { doc_type: 'invoice', confidence: 0.98, extracted_fields: null, raw_extraction: null, user_directive: null } },
  { id: 'd2', status: 'processed', asset_id: 'a2', source_id: 's2', source_filename: 'warranty_washer.pdf', source_uploaded_at: '2026-08-02T10:00:00Z', owner_household_id: null, created_at: '2026-08-02T10:00:00Z', updated_at: '2026-08-02T10:00:00Z', data: { doc_type: 'warranty', confidence: 0.91, extracted_fields: null, raw_extraction: null, user_directive: null } },
  { id: 'd3', status: 'in_review', asset_id: null, source_id: 's3', source_filename: 'receipt_pending.jpg', source_uploaded_at: '2026-08-03T10:00:00Z', owner_household_id: null, created_at: '2026-08-03T10:00:00Z', updated_at: '2026-08-03T10:00:00Z', data: { doc_type: 'receipt', confidence: 0.4, extracted_fields: null, raw_extraction: null, user_directive: null } },
  { id: 'd4', status: 'failed', asset_id: null, source_id: 's4', source_filename: 'scanned_invoice.pdf', source_uploaded_at: '2026-08-04T10:00:00Z', owner_household_id: null, created_at: '2026-08-04T10:00:00Z', updated_at: '2026-08-04T10:00:00Z', data: { doc_type: 'other', confidence: 0.1, extracted_fields: null, raw_extraction: null, user_directive: null } },
  { id: 'd5', status: 'asset_less', asset_id: null, source_id: 's5', source_filename: 'warranty_note.txt', source_uploaded_at: '2026-08-05T10:00:00Z', owner_household_id: null, created_at: '2026-08-05T10:00:00Z', updated_at: '2026-08-05T10:00:00Z', data: { doc_type: 'warranty', confidence: 0.7, extracted_fields: null, raw_extraction: null, user_directive: null } },
];

const accounts = [
  { id: 'acc1', owner_household_id: null, created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z', data: { name: 'HDFC Savings', account_type: 'bank', currency: 'INR', institution: 'HDFC', external_descriptor: 'SAV-123', balance: '45230.50' } },
  { id: 'acc2', owner_household_id: null, created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z', data: { name: 'PayPal', account_type: 'wallet', currency: 'USD', institution: null, external_descriptor: null, balance: '1280.00' } },
  { id: 'acc3', owner_household_id: null, created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z', data: { name: 'Cash', account_type: 'cash', currency: 'INR', institution: null, external_descriptor: null, balance: '3500.00' } },
  { id: 'acc4', owner_household_id: null, created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z', data: { name: 'ICICI Credit', account_type: 'credit_card', currency: 'INR', institution: 'ICICI', external_descriptor: null, balance: '-12450.75' } },
];

const movements = [
  { id: 'm1', owner_household_id: null, source_account_id: 'acc1', destination_account_id: null, import_batch_id: null, linked_document_id: null, created_at: '2026-08-15T10:00:00Z', updated_at: '2026-08-15T10:00:00Z', data: { kind: 'expense', amount: '1200.00', currency: 'INR', occurred_on: '2026-08-15', recorded_at: '2026-08-15T10:00:00Z', description: 'Groceries', origin: 'manual', import_line: null, external_reference: null, link_creator: null, link_conflicting: false } },
  { id: 'm2', owner_household_id: null, source_account_id: null, destination_account_id: 'acc1', import_batch_id: 'b2', linked_document_id: null, created_at: '2026-08-01T10:00:00Z', updated_at: '2026-08-01T10:00:00Z', data: { kind: 'income', amount: '50000.00', currency: 'INR', occurred_on: '2026-08-01', recorded_at: '2026-08-01T10:00:00Z', description: 'Salary', origin: 'import', import_line: 1, external_reference: 'ref1', link_creator: null, link_conflicting: false } },
  { id: 'm3', owner_household_id: null, source_account_id: 'acc1', destination_account_id: 'acc3', import_batch_id: null, linked_document_id: null, created_at: '2026-08-20T10:00:00Z', updated_at: '2026-08-20T10:00:00Z', data: { kind: 'transfer', amount: '5000.00', currency: 'INR', occurred_on: '2026-08-20', recorded_at: '2026-08-20T10:00:00Z', description: 'Bank to Cash', origin: 'manual', import_line: null, external_reference: null, link_creator: null, link_conflicting: false } },
  { id: 'm4', owner_household_id: null, source_account_id: 'acc2', destination_account_id: null, import_batch_id: null, linked_document_id: null, created_at: '2026-08-25T10:00:00Z', updated_at: '2026-08-25T10:00:00Z', data: { kind: 'expense', amount: '9.99', currency: 'USD', occurred_on: '2026-08-25', recorded_at: '2026-08-25T10:00:00Z', description: 'Spotify', origin: 'manual', import_line: null, external_reference: null, link_creator: null, link_conflicting: false } },
  { id: 'm5', owner_household_id: null, source_account_id: 'acc4', destination_account_id: null, import_batch_id: null, linked_document_id: null, created_at: '2026-09-01T10:00:00Z', updated_at: '2026-09-01T10:00:00Z', data: { kind: 'expense', amount: '2500.00', currency: 'INR', occurred_on: '2026-09-01', recorded_at: '2026-09-01T10:00:00Z', description: 'Electricity', origin: 'manual', import_line: null, external_reference: null, link_creator: null, link_conflicting: false } },
  { id: 'm6', owner_household_id: null, source_account_id: 'acc1', destination_account_id: null, import_batch_id: null, linked_document_id: 'd1', created_at: '2026-09-02T10:00:00Z', updated_at: '2026-09-02T10:00:00Z', data: { kind: 'expense', amount: '3500.00', currency: 'INR', occurred_on: '2026-09-02', recorded_at: '2026-09-02T10:00:00Z', description: 'Refilled cash', origin: 'manual', import_line: null, external_reference: null, link_creator: 'manual', link_conflicting: true } },
  { id: 'm7', owner_household_id: null, source_account_id: null, destination_account_id: 'acc1', import_batch_id: null, linked_document_id: null, created_at: '2026-09-03T10:00:00Z', updated_at: '2026-09-03T10:00:00Z', data: { kind: 'income', amount: '1000.00', currency: 'INR', occurred_on: '2026-09-03', recorded_at: '2026-09-03T10:00:00Z', description: 'Dividend', origin: 'manual', import_line: null, external_reference: null, link_creator: null, link_conflicting: false } },
  { id: 'm8', owner_household_id: null, source_account_id: 'acc3', destination_account_id: null, import_batch_id: null, linked_document_id: null, created_at: '2026-09-03T10:00:00Z', updated_at: '2026-09-03T10:00:00Z', data: { kind: 'expense', amount: '500.00', currency: 'INR', occurred_on: '2026-09-03', recorded_at: '2026-09-03T10:00:00Z', description: 'Coffee', origin: 'manual', import_line: null, external_reference: null, link_creator: null, link_conflicting: false } },
];

const importBatches = [
  { id: 'b1', account_id: 'acc1', source: { id: 's1', filename: 'aug.csv', content_type: 'text/csv', size: 1024, sha256: 'abc', uploaded_at: '2026-09-01T10:00:00Z' }, created_at: '2026-09-01T10:00:00Z', updated_at: '2026-09-01T10:00:00Z', data: { state: 'preview', filename: 'aug.csv', format: 'csv', line_count_valid: 8, line_count_duplicate: 1, line_count_possible_dup: 1, line_count_error: 0, lines: Array.from({ length: 10 }).map((_, i) => ({ line_ref: i, raw_line: 'row', status: 'valid' })) } },
  { id: 'b2', account_id: 'acc2', source: { id: 's2', filename: 'jul.csv', content_type: 'text/csv', size: 2048, sha256: 'def', uploaded_at: '2026-08-01T10:00:00Z' }, created_at: '2026-08-01T10:00:00Z', updated_at: '2026-08-01T10:00:00Z', data: { state: 'committed', filename: 'jul.csv', format: 'csv', line_count_valid: 5, line_count_duplicate: 0, line_count_possible_dup: 0, line_count_error: 0, lines: null } },
  { id: 'b3', account_id: 'acc4', source: { id: 's3', filename: 'jul_cc.csv', content_type: 'text/csv', size: 512, sha256: 'ghi', uploaded_at: '2026-08-01T10:00:00Z' }, created_at: '2026-08-01T10:00:00Z', updated_at: '2026-08-01T10:00:00Z', data: { state: 'discarded', filename: 'jul_cc.csv', format: 'csv', line_count_valid: 0, line_count_duplicate: 0, line_count_possible_dup: 0, line_count_error: 10, lines: null } },
];

export const handlers = [
  http.get(`/api/users/:uid/assets`, ({ params }) => {
    if (params.uid !== userId) return new HttpResponse(null, { status: 404 });
    return HttpResponse.json(assets);
  }),
  http.get(`/api/users/:uid/assets/:assetId`, ({ params }) => {
    if (params.uid !== userId) return new HttpResponse(null, { status: 404 });
    const asset = assets.find(a => a.id === params.assetId);
    return asset ? HttpResponse.json(asset) : new HttpResponse(null, { status: 404 });
  }),
  http.get(`/api/users/:uid/assets/:assetId/documents`, ({ params }) => {
    if (params.uid !== userId) return new HttpResponse(null, { status: 404 });
    return HttpResponse.json(documents.filter(d => d.source_filename.includes(params.assetId as string)));
  }),
  http.post(`/api/users/:uid/documents`, () => new HttpResponse(JSON.stringify(documents[0]), { status: 201 })),
  http.get(`/api/users/:uid/documents`, ({ params, request }) => {
    if (params.uid !== userId) return new HttpResponse(null, { status: 404 });
    const url = new URL(request.url);
    const status = url.searchParams.get('status');
    const q = url.searchParams.get('q');
    let filtered = documentRows;
    if (status) filtered = filtered.filter(d => d.status === status);
    if (q) filtered = filtered.filter(d => d.source_filename.toLowerCase().includes(q.toLowerCase()));
    return HttpResponse.json(filtered);
  }),
  http.post(`/api/users/:uid/documents/:id/reprocess`, ({ params }) => {
    if (params.uid !== userId) return new HttpResponse(null, { status: 404 });
    const doc = documentRows.find(d => d.id === params.id);
    if (!doc) return new HttpResponse(null, { status: 404 });
    if (doc.status === 'in_review') {
      return HttpResponse.json({ error: 'Document reprocess already in flight' }, { status: 409 });
    }
    return HttpResponse.json({ ...doc, status: 'in_review' }, { status: 202 });
  }),
  http.delete(`/api/users/:uid/documents/:id`, ({ params }) => {
    if (params.uid !== userId) return new HttpResponse(null, { status: 404 });
    const doc = documentRows.find(d => d.id === params.id);
    if (!doc) return new HttpResponse(null, { status: 404 });
    return new HttpResponse(null, { status: 204 });
  }),
  http.get(`/api/users/:uid/finance/accounts`, ({ params }) => {
    if (params.uid !== userId) return new HttpResponse(null, { status: 404 });
    return HttpResponse.json(accounts);
  }),
  http.post(`/api/users/:uid/finance/accounts`, () => HttpResponse.json(accounts[0])),
  http.get(`/api/users/:uid/finance/movements`, ({ params, request }) => {
    if (params.uid !== userId) return new HttpResponse(null, { status: 404 });
    const url = new URL(request.url);
    const accountId = url.searchParams.get('account_id');
    const from = url.searchParams.get('from');
    const to = url.searchParams.get('to');
    
    let filtered = movements;
    if (accountId) filtered = filtered.filter(m => m.source_account_id === accountId || m.destination_account_id === accountId);
    if (from) filtered = filtered.filter(m => m.data.occurred_on >= from);
    if (to) filtered = filtered.filter(m => m.data.occurred_on <= to);
    
    return HttpResponse.json(filtered);
  }),
  http.post(`/api/users/:uid/finance/movements`, () => HttpResponse.json(movements[0])),
  http.get(`/api/users/:uid/finance/movements/:id`, ({ params }) => {
    if (params.uid !== userId) return new HttpResponse(null, { status: 404 });
    const movement = movements.find(m => m.id === params.id);
    return movement ? HttpResponse.json(movement) : new HttpResponse(null, { status: 404 });
  }),
  http.patch(`/api/users/:uid/finance/movements/:id`, ({ params }) => {
    if (params.uid !== userId) return new HttpResponse(null, { status: 404 });
    return HttpResponse.json(movements[0]);
  }),
  http.delete(`/api/users/:uid/finance/movements/:id`, ({ params }) => {
    if (params.uid !== userId) return new HttpResponse(null, { status: 404 });
    const movement = movements.find(m => m.id === params.id);
    if (movement?.data.origin === 'import') return new HttpResponse(null, { status: 409 });
    return new HttpResponse(null, { status: 204 });
  }),
  http.post(`/api/users/:uid/finance/movements/:id/link`, () => new HttpResponse(null, { status: 204 })),
  http.delete(`/api/users/:uid/finance/movements/:id/link`, () => new HttpResponse(null, { status: 204 })),
  http.get(`/api/users/:uid/finance/import-batches`, ({ params }) => {
    if (params.uid !== userId) return new HttpResponse(null, { status: 404 });
    return HttpResponse.json(importBatches);
  }),
  http.get(`/api/users/:uid/finance/import-batches/:id`, ({ params }) => {
    if (params.uid !== userId) return new HttpResponse(null, { status: 404 });
    const batch = importBatches.find(b => b.id === params.id);
    return batch ? HttpResponse.json(batch) : new HttpResponse(null, { status: 404 });
  }),
  http.post(`/api/users/:uid/finance/import-batches/:id/commit`, () => HttpResponse.json({ created: 7, skipped: 3 })),
  http.post(`/api/users/:uid/finance/import-batches/:id/discard`, () => HttpResponse.json(importBatches[0])),
  http.post(`/api/users/:uid/finance/import-batches`, () => HttpResponse.json(importBatches[0], { status: 201 })),
];

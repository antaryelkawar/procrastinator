import { http, HttpResponse } from 'msw';

const userId = 'alice';

const assets = [
  { id: 'a1', brand: 'Samsung', model: 'Galaxy S23', serial_number: 'SN1001', purchase_date: '2026-08-01T00:00:00Z', warranty_end: '2027-08-01T00:00:00Z', price: '75000.00', currency: 'INR', doc_type: 'invoice', metadata: {}, created_at: '2026-08-01T00:00:00Z', updated_at: '2026-08-01T00:00:00Z' },
  { id: 'a2', brand: 'LG', model: 'Washing Machine', serial_number: 'SN2002', purchase_date: '2024-01-01T00:00:00Z', warranty_end: '2025-01-01T00:00:00Z', price: '35000.00', currency: 'INR', doc_type: 'warranty', metadata: {}, created_at: '2026-08-01T00:00:00Z', updated_at: '2026-08-01T00:00:00Z' },
  { id: 'a3', brand: 'Sony', model: 'Bravia TV', serial_number: 'SN3003', purchase_date: '2025-06-01T00:00:00Z', warranty_end: '2026-06-01T00:00:00Z', price: '120000.00', currency: 'EUR', doc_type: 'amc', metadata: {}, created_at: '2026-08-05T00:00:00Z', updated_at: '2026-08-05T00:00:00Z' },
  { id: 'a4', brand: 'Bosch', model: 'Dishwasher', serial_number: 'SN4004', purchase_date: '2026-08-10T00:00:00Z', warranty_end: '2028-08-10T00:00:00Z', price: '45000.00', currency: 'INR', doc_type: 'invoice', metadata: {}, created_at: '2026-08-10T00:00:00Z', updated_at: '2026-08-10T00:00:00Z' },
];

const documents = [
  ...assets.map((a, i) => ({ id: `d${i * 2 + 1}`, doc_type: a.doc_type, source_filename: `doc_${a.id}_1.pdf`, source_uploaded_at: a.created_at, created_at: a.created_at })),
  ...assets.map((a, i) => ({ id: `d${i * 2 + 2}`, doc_type: 'other', source_filename: `doc_${a.id}_2.pdf`, source_uploaded_at: a.created_at, created_at: a.created_at })),
];

const accounts = [
  { id: 'acc1', name: 'HDFC Savings', type: 'bank', currency: 'INR', institution: 'HDFC', external_descriptor: 'SAV-123', balance: '45230.50', created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' },
  { id: 'acc2', name: 'PayPal', type: 'wallet', currency: 'USD', balance: '1280.00', created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' },
  { id: 'acc3', name: 'Cash', type: 'cash', currency: 'INR', balance: '3500.00', created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' },
  { id: 'acc4', name: 'ICICI Credit', type: 'credit_card', currency: 'INR', institution: 'ICICI', balance: '-12450.75', created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' },
];

const movements = [
  { id: 'm1', kind: 'expense', amount: '1200.00', currency: 'INR', occurred_on: '2026-08-15', recorded_at: '2026-08-15T10:00:00Z', description: 'Groceries', origin: 'manual', source_account_id: 'acc1', link_conflicting: false, created_at: '2026-08-15T10:00:00Z', updated_at: '2026-08-15T10:00:00Z' },
  { id: 'm2', kind: 'income', amount: '50000.00', currency: 'INR', occurred_on: '2026-08-01', recorded_at: '2026-08-01T10:00:00Z', description: 'Salary', origin: 'import', destination_account_id: 'acc1', import_batch_id: 'b2', import_line: 1, external_reference: 'ref1', link_conflicting: false, created_at: '2026-08-01T10:00:00Z', updated_at: '2026-08-01T10:00:00Z' },
  { id: 'm3', kind: 'transfer', amount: '5000.00', currency: 'INR', occurred_on: '2026-08-20', recorded_at: '2026-08-20T10:00:00Z', description: 'Bank to Cash', origin: 'manual', source_account_id: 'acc1', destination_account_id: 'acc3', link_conflicting: false, created_at: '2026-08-20T10:00:00Z', updated_at: '2026-08-20T10:00:00Z' },
  { id: 'm4', kind: 'expense', amount: '9.99', currency: 'USD', occurred_on: '2026-08-25', recorded_at: '2026-08-25T10:00:00Z', description: 'Spotify', origin: 'manual', source_account_id: 'acc2', link_conflicting: false, created_at: '2026-08-25T10:00:00Z', updated_at: '2026-08-25T10:00:00Z' },
  { id: 'm5', kind: 'expense', amount: '2500.00', currency: 'INR', occurred_on: '2026-09-01', recorded_at: '2026-09-01T10:00:00Z', description: 'Electricity', origin: 'manual', source_account_id: 'acc4', link_conflicting: false, created_at: '2026-09-01T10:00:00Z', updated_at: '2026-09-01T10:00:00Z' },
  { id: 'm6', kind: 'expense', amount: '3500.00', currency: 'INR', occurred_on: '2026-09-02', recorded_at: '2026-09-02T10:00:00Z', description: 'Refilled cash', origin: 'manual', source_account_id: 'acc1', linked_document_id: 'd1', link_creator: 'manual', link_conflicting: true, created_at: '2026-09-02T10:00:00Z', updated_at: '2026-09-02T10:00:00Z' },
  { id: 'm7', kind: 'income', amount: '1000.00', currency: 'INR', occurred_on: '2026-09-03', recorded_at: '2026-09-03T10:00:00Z', description: 'Dividend', origin: 'manual', destination_account_id: 'acc1', link_conflicting: false, created_at: '2026-09-03T10:00:00Z', updated_at: '2026-09-03T10:00:00Z' },
  { id: 'm8', kind: 'expense', amount: '500.00', currency: 'INR', occurred_on: '2026-09-03', recorded_at: '2026-09-03T10:00:00Z', description: 'Coffee', origin: 'manual', source_account_id: 'acc3', link_conflicting: false, created_at: '2026-09-03T10:00:00Z', updated_at: '2026-09-03T10:00:00Z' },
];

const importBatches = [
  { id: 'b1', state: 'preview', account_id: 'acc1', source: { id: 's1', filename: 'aug.csv', content_type: 'text/csv', size: 1024, sha256: 'abc', uploaded_at: '2026-09-01T10:00:00Z' }, filename: 'aug.csv', format: 'csv', line_count_valid: 8, line_count_duplicate: 1, line_count_possible_dup: 1, line_count_error: 0, created_at: '2026-09-01T10:00:00Z', updated_at: '2026-09-01T10:00:00Z', lines: Array.from({ length: 10 }).map((_, i) => ({ line_ref: i, raw_line: 'row', status: 'valid' })) },
  { id: 'b2', state: 'committed', account_id: 'acc2', source: { id: 's2', filename: 'jul.csv', content_type: 'text/csv', size: 2048, sha256: 'def', uploaded_at: '2026-08-01T10:00:00Z' }, filename: 'jul.csv', format: 'csv', line_count_valid: 5, line_count_duplicate: 0, line_count_possible_dup: 0, line_count_error: 0, created_at: '2026-08-01T10:00:00Z', updated_at: '2026-08-01T10:00:00Z', lines: null },
  { id: 'b3', state: 'discarded', account_id: 'acc4', source: { id: 's3', filename: 'jul_cc.csv', content_type: 'text/csv', size: 512, sha256: 'ghi', uploaded_at: '2026-08-01T10:00:00Z' }, filename: 'jul_cc.csv', format: 'csv', line_count_valid: 0, line_count_duplicate: 0, line_count_possible_dup: 0, line_count_error: 10, created_at: '2026-08-01T10:00:00Z', updated_at: '2026-08-01T10:00:00Z', lines: null },
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
    if (from) filtered = filtered.filter(m => m.occurred_on >= from);
    if (to) filtered = filtered.filter(m => m.occurred_on <= to);
    
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
    if (movement?.origin === 'import') return new HttpResponse(null, { status: 409 });
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

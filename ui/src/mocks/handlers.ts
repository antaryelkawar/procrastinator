import { http, HttpResponse } from 'msw';

export const handlers = [
  http.get('/api/users/:userId/assets', () => {
    return HttpResponse.json([
      {
        id: 'a1',
        brand: 'BrandX',
        model: 'ModelY',
        serial_number: 'SN123',
        warranty_end: '2025-01-01T00:00:00Z',
        doc_type: 'invoice',
        created_at: '2026-08-01T00:00:00Z',
        updated_at: '2026-08-01T00:00:00Z',
        metadata: {},
        scope_type: 'personal',
      },
      {
        id: 'a2',
        brand: 'Old',
        model: 'Model',
        serial_number: 'SN',
        warranty_end: '2026-01-01T00:00:00Z',
        doc_type: 'warranty',
        created_at: '2026-08-01T00:00:00Z',
        updated_at: '2026-08-01T00:00:00Z',
        metadata: {},
        scope_type: 'personal',
      },
    ]);
  }),
  http.get('/api/finance/accounts', () => {
    return HttpResponse.json([
      {
        id: 'acc1',
        name: 'Main Bank',
        type: 'bank',
        currency: 'USD',
        balance: '150',
        created_at: '2026-08-01T00:00:00Z',
        updated_at: '2026-08-01T00:00:00Z',
      },
    ]);
  }),
  http.post('/api/finance/accounts', async ({ request }) => {
    const body = (await request.json()) as any;
    if (!body.name || !body.type || !body.currency) {
      return new HttpResponse(JSON.stringify({ reason: 'Validation failed' }), { status: 400 });
    }
    return HttpResponse.json({
      id: 'acc2',
      name: body.name,
      type: body.type,
      currency: body.currency,
      institution: body.institution,
      external_descriptor: body.external_descriptor,
      balance: '0',
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    });
  }),
];

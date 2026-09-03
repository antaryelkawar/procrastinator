import '@testing-library/jest-dom/vitest';
import * as matchers from 'vitest-axe/matchers';
import 'vitest-axe/extend-expect';
import { expect, beforeAll, afterEach, afterAll } from 'vitest';
import { server } from '../mocks/server';

expect.extend(matchers);

beforeAll(() => server.listen());
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

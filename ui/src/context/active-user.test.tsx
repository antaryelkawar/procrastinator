import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, fireEvent } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ActiveUserProvider, useActiveUser } from './active-user';

// Mock localStorage
let localStorageMock = (function () {
  let store: { [key: string]: string } = {};
  return {
    getItem: function (key: string): string | null {
      return store[key] || null;
    },
    setItem: function (key: string, value: string) {
      store[key] = value;
    },
    removeItem: function (key: string) {
      delete store[key];
    },
    clear: function () {
      store = {};
    },
  };
})();

Object.defineProperty(window, 'localStorage', {
  value: localStorageMock,
});

// Mock QueryClient with retries off so tests don't retry failing fetches.
function makeQueryClient(): QueryClient {
  return new QueryClient({ defaultOptions: { queries: { retry: false } } });
}

// Helper component to use the hook and render its output
function TestComponent() {
  const { activeUser, setActiveUser, clearActiveUser } = useActiveUser();
  return (
    <div>
      <div data-testid="active-user">{activeUser || 'null'}</div>
      <button onClick={() => setActiveUser('testUser')}>Set User</button>
      <button onClick={() => setActiveUser('invalid-user!')}>Set Invalid User</button>
      <button onClick={() => clearActiveUser()}>Clear User</button>
    </div>
  );
}

// Wrapper to provide QueryClient to the test component
function renderWithClient(queryClient: QueryClient) {
  return render(
    <QueryClientProvider client={queryClient}>
      <ActiveUserProvider queryClient={queryClient}>
        <TestComponent />
      </ActiveUserProvider>
    </QueryClientProvider>,
  );
}

describe('ActiveUserProvider', () => {
  beforeEach(() => {
    // Reset the localStorage mock before each test.
    localStorageMock.clear();
  });

  it('should provide the default activeUser as null if not in localStorage', () => {
    const { getByTestId } = renderWithClient(makeQueryClient());
    expect(getByTestId('active-user')).toHaveTextContent('null');
  });

  it('should load activeUser from localStorage on initial render', () => {
    localStorageMock.setItem('activeUser', 'persistedUser');
    const { getByTestId } = renderWithClient(makeQueryClient());
    expect(getByTestId('active-user')).toHaveTextContent('persistedUser');
  });

  it('should set activeUser and persist it to localStorage', () => {
    const { getByText, getByTestId } = renderWithClient(makeQueryClient());
    fireEvent.click(getByText('Set User'));
    expect(getByTestId('active-user')).toHaveTextContent('testUser');
    expect(localStorageMock.getItem('activeUser')).toBe('testUser');
  });

  it('should not set activeUser if the format is invalid', () => {
    const { getByTestId, getByText } = renderWithClient(makeQueryClient());
    fireEvent.click(getByText('Set Invalid User'));
    expect(getByTestId('active-user')).toHaveTextContent('null'); // Should remain null or its previous value
    expect(localStorageMock.getItem('activeUser')).toBeNull();
  });

  it('should clear activeUser from state and localStorage', () => {
    localStorageMock.setItem('activeUser', 'userToClear');
    const { getByText, getByTestId } = renderWithClient(makeQueryClient());
    expect(getByTestId('active-user')).toHaveTextContent('userToClear');
    fireEvent.click(getByText('Clear User'));
    expect(getByTestId('active-user')).toHaveTextContent('null');
    expect(localStorageMock.getItem('activeUser')).toBeNull();
  });

  it('should clear the query cache when setActiveUser is called', () => {
    const queryClient = makeQueryClient();
    const queryClientSpy = vi.spyOn(queryClient, 'clear');

    const { getByText } = renderWithClient(queryClient);
    fireEvent.click(getByText('Set User'));

    expect(queryClientSpy).toHaveBeenCalledTimes(1);
  });

  it('should clear the query cache when clearActiveUser is called', () => {
    const queryClient = makeQueryClient();
    const queryClientSpy = vi.spyOn(queryClient, 'clear');

    localStorageMock.setItem('activeUser', 'userToClear');
    const { getByText } = renderWithClient(queryClient);
    fireEvent.click(getByText('Clear User'));

    expect(queryClientSpy).toHaveBeenCalledTimes(1);
  });
});

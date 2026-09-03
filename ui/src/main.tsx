import React from 'react';
import ReactDOM from 'react-dom/client';
import './index.css';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { BrowserRouter } from 'react-router-dom';
import { ActiveUserProvider } from './context/active-user';
import { AppRoutes } from './router';

async function enableMocking() {
  if (!import.meta.env.DEV) {
    return;
  }
  try {
    const { worker } = await import('./mocks/browser');
    await worker.start({ onUnhandledRequest: 'bypass' });
  } catch (err) {
    console.warn('[MSW] Mocking disabled — falling through to real API.', err);
  }
}

const queryClient = new QueryClient();

enableMocking().then(() => {
  ReactDOM.createRoot(document.getElementById('root')!).render(
    <React.StrictMode>
      <QueryClientProvider client={queryClient}>
        <BrowserRouter>
          <ActiveUserProvider queryClient={queryClient}>
            <AppRoutes />
          </ActiveUserProvider>
        </BrowserRouter>
      </QueryClientProvider>
    </React.StrictMode>,
  );
});

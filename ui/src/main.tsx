import React from 'react';
import ReactDOM from 'react-dom/client';
import './index.css';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { BrowserRouter } from 'react-router';
import { ActiveUserProvider } from './context/active-user';
import { ThemeProvider } from './context/theme-provider';
import { AppRoutes } from './router';

async function enableMocking() {
  if (import.meta.env.VITE_USE_MSW !== 'true') {
    return;
  }
  const { worker } = await import('./mocks/browser');
  await worker.start({ onUnhandledRequest: 'bypass' });
}

const queryClient = new QueryClient();

enableMocking().then(() => {
  ReactDOM.createRoot(document.getElementById('root')!).render(
    <React.StrictMode>
      <ThemeProvider>
        <QueryClientProvider client={queryClient}>
          <BrowserRouter>
            <ActiveUserProvider queryClient={queryClient}>
              <AppRoutes />
            </ActiveUserProvider>
          </BrowserRouter>
        </QueryClientProvider>
      </ThemeProvider>
    </React.StrictMode>,
  );
});

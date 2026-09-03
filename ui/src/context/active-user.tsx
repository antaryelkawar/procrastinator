
import { createContext, useContext, useState, ReactNode } from 'react';
import { QueryClient } from '@tanstack/react-query';

interface ActiveUserContextType {
  activeUser: string | null;
  setActiveUser: (user: string) => boolean;
  clearActiveUser: () => void;
}

const ActiveUserContext = createContext<ActiveUserContextType | undefined>(undefined);

const USER_REGEX = /^[A-Za-z0-9_-]{1,64}$/;

export const ActiveUserProvider = ({ children, queryClient }: { children: ReactNode; queryClient: QueryClient }) => {
  const [activeUser, setInternalActiveUser] = useState<string | null>(() => {
    try {
      return localStorage.getItem('activeUser');
    } catch (error) {
      console.error('Error accessing localStorage:', error);
      return null;
    }
  });

  const setActiveUser = (user: string): boolean => {
    if (USER_REGEX.test(user)) {
      try {
        localStorage.setItem('activeUser', user);
        setInternalActiveUser(user);
        // Clear query cache when switching users
        queryClient.clear();
        return true;
      } catch (error) {
        console.error('Error setting active user in localStorage:', error);
        return false;
      }
    } else {
      console.error('Invalid user format. Use alphanumeric characters, underscores, and hyphens, 1-64 characters long.');
      return false;
    }
  };

  const clearActiveUser = () => {
    try {
      localStorage.removeItem('activeUser');
      setInternalActiveUser(null);
      // Clear query cache when clearing active user
      queryClient.clear();
    } catch (error) {
      console.error('Error clearing active user from localStorage:', error);
    }
  };

  return (
    <ActiveUserContext.Provider value={{ activeUser, setActiveUser, clearActiveUser }}>
      {children}
    </ActiveUserContext.Provider>
  );
};

/**
 * Returns the active-user context. While no `ActiveUserProvider` is mounted
 * (e.g. in bare router/shell tests) the hook degrades to a no-op instead of
 * throwing: `activeUser` stays `null`, so every query built on it is disabled
 * and the screens render their empty states. The app root always provides the
 * real context (see `main.tsx`).
 */
export const useActiveUser = (): ActiveUserContextType => {
  const context = useContext(ActiveUserContext);
  if (context !== undefined) {
    return context;
  }
  return {
    activeUser: null,
    setActiveUser: () => false,
    clearActiveUser: () => undefined,
  };
};

// Mock QueryClient for initial setup if not provided elsewhere
// In a real app, this would be imported and provided at a higher level
// const mockQueryClient = new QueryClient();

// Example of how to use the provider (typically in your App.tsx or index.tsx)
// export const App = () => (
//   <ActiveUserProvider queryClient={mockQueryClient}>
//     {/* Your app components */}
//   </ActiveUserProvider>
// );

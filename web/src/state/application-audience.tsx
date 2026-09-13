import { createContext, useContext, useLayoutEffect, useMemo, type ReactNode } from "react";

const AudienceContext = createContext({ active: true });

// Parent layout cleanup runs before descendant passive cleanup. A queued
// control may flush when changing routes, but cannot outlive its login tree.
export function ApplicationAudience({ children }: { children: ReactNode }) {
  const lifetime = useMemo(() => ({ active: true }), []);
  useLayoutEffect(() => {
    lifetime.active = true;
    return () => { lifetime.active = false; };
  }, [lifetime]);
  return <AudienceContext.Provider value={lifetime}>{children}</AudienceContext.Provider>;
}

export function useApplicationAudience() { return useContext(AudienceContext); }

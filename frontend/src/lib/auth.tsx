"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import * as client from "./api";
import type { Role, Session, User } from "./types";

type AuthState =
  | { status: "loading" }
  | { status: "signedOut" }
  | { status: "signedIn"; user: User; role: Role };

type Auth = AuthState & {
  signIn: (email: string, password: string) => Promise<void>;
  signOut: () => Promise<void>;
  // can is a convenience for hiding controls. The API enforces permissions regardless.
  can: (permission: string) => boolean;
};

const AuthContext = createContext<Auth | null>(null);

const signedIn = (s: Session): AuthState => ({ status: "signedIn", user: s.user, role: s.role });

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [state, setState] = useState<AuthState>({ status: "loading" });

  // On first load, try to resume the session from the refresh cookie.
  useEffect(() => {
    let alive = true;
    client.refreshSession().then((session) => {
      if (alive) setState(session ? signedIn(session) : { status: "signedOut" });
    });
    client.setSessionLostHandler(() => setState({ status: "signedOut" }));
    return () => {
      alive = false;
      client.setSessionLostHandler(null);
    };
  }, []);

  const signIn = useCallback(async (email: string, password: string) => {
    setState(signedIn(await client.login(email, password)));
  }, []);

  const signOut = useCallback(async () => {
    await client.logout();
    setState({ status: "signedOut" });
  }, []);

  const value = useMemo<Auth>(
    () => ({
      ...state,
      signIn,
      signOut,
      can: (permission) => state.status === "signedIn" && state.role.permissions.includes(permission),
    }),
    [state, signIn, signOut],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): Auth {
  const auth = useContext(AuthContext);
  if (!auth) throw new Error("useAuth must be used inside AuthProvider");
  return auth;
}

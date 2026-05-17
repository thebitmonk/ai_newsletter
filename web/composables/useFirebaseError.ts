// Translates Firebase Auth error codes into friendly UI messages so each
// page doesn't carry its own string table. Unknown codes fall back to the
// raw Firebase message — surfacing the underlying error beats hiding it.

import type { FirebaseError } from "firebase/app";

const MESSAGES: Record<string, string> = {
  "auth/email-already-in-use":
    "An account with this email already exists. Try signing in instead.",
  "auth/invalid-email": "That email address doesn't look valid.",
  "auth/missing-email": "Please enter your email address.",
  "auth/weak-password": "Password must be at least 6 characters.",
  "auth/wrong-password": "Email or password is incorrect.",
  "auth/user-not-found": "Email or password is incorrect.",
  "auth/invalid-credential": "Email or password is incorrect.",
  "auth/invalid-login-credentials": "Email or password is incorrect.",
  "auth/too-many-requests":
    "Too many attempts. Try again in a few minutes, or reset your password.",
  "auth/popup-closed-by-user": "Sign-in window closed. Please try again.",
  "auth/popup-blocked":
    "Your browser blocked the sign-in popup. Allow popups for this site and try again.",
  "auth/network-request-failed": "Network error — check your connection.",
};

export function friendlyFirebaseError(err: unknown): string {
  if (err && typeof err === "object" && "code" in err) {
    const fe = err as FirebaseError;
    return MESSAGES[fe.code] ?? fe.message ?? String(err);
  }
  return err instanceof Error ? err.message : String(err);
}

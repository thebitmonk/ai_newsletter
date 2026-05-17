// Thin reactive wrapper around the Firebase Auth JS SDK. Single source of
// truth for the current user + the auto-refreshing ID token used by useApi.

import {
  GoogleAuthProvider,
  createUserWithEmailAndPassword,
  onAuthStateChanged,
  onIdTokenChanged,
  sendEmailVerification,
  sendPasswordResetEmail,
  signInWithEmailAndPassword,
  signInWithPopup,
  signOut as fbSignOut,
  type User,
  type UserCredential,
} from "firebase/auth";

interface UseFirebaseAuth {
  currentUser: Ref<User | null>;
  idToken: Ref<string | null>;
  signUpWithEmail: (email: string, password: string) => Promise<UserCredential>;
  signInWithEmail: (email: string, password: string) => Promise<UserCredential>;
  signInWithGoogle: () => Promise<UserCredential>;
  signOut: () => Promise<void>;
  sendVerificationEmail: () => Promise<void>;
  sendPasswordResetEmail: (email: string) => Promise<void>;
  refreshIdToken: () => Promise<string | null>;
}

// Module-scoped singletons so every composable consumer shares one reactive
// state — without this we'd get N parallel onAuthStateChanged listeners.
let _currentUser: Ref<User | null> | null = null;
let _idToken: Ref<string | null> | null = null;
let _initialised = false;

export function useFirebaseAuth(): UseFirebaseAuth {
  if (!_currentUser) _currentUser = ref<User | null>(null);
  if (!_idToken) _idToken = ref<string | null>(null);

  // Lazy SDK wiring — only on the client, only once per process.
  if (import.meta.client && !_initialised) {
    const { $firebaseAuth } = useNuxtApp();
    onAuthStateChanged($firebaseAuth, (u) => {
      _currentUser!.value = u;
    });
    onIdTokenChanged($firebaseAuth, async (u) => {
      _idToken!.value = u ? await u.getIdToken() : null;
    });
    _initialised = true;
  }

  const auth = () => useNuxtApp().$firebaseAuth;

  return {
    currentUser: _currentUser,
    idToken: _idToken,
    signUpWithEmail: (email, password) =>
      createUserWithEmailAndPassword(auth(), email, password),
    signInWithEmail: (email, password) =>
      signInWithEmailAndPassword(auth(), email, password),
    signInWithGoogle: () => signInWithPopup(auth(), new GoogleAuthProvider()),
    signOut: () => fbSignOut(auth()),
    sendVerificationEmail: async () => {
      const u = auth().currentUser;
      if (!u) throw new Error("not signed in");
      await sendEmailVerification(u);
    },
    sendPasswordResetEmail: (email) => sendPasswordResetEmail(auth(), email),
    refreshIdToken: async () => {
      const u = auth().currentUser;
      if (!u) return null;
      const t = await u.getIdToken(true);
      _idToken!.value = t;
      return t;
    },
  };
}

// Initialise Firebase Auth on the client. Server-side never touches Firebase
// — auth state is per-browser, so SSR for protected pages renders a "loading"
// shell that hydrates with the real auth state on the client.

import { initializeApp, type FirebaseApp } from "firebase/app";
import { getAuth, connectAuthEmulator, type Auth } from "firebase/auth";

declare module "#app" {
  interface NuxtApp {
    $firebaseApp: FirebaseApp;
    $firebaseAuth: Auth;
  }
}

export default defineNuxtPlugin(() => {
  const config = useRuntimeConfig().public;
  if (!config.firebaseApiKey || !config.firebaseProjectId) {
    throw new Error(
      "Firebase config missing — set NUXT_PUBLIC_FIREBASE_API_KEY and NUXT_PUBLIC_FIREBASE_PROJECT_ID in .env",
    );
  }

  const app = initializeApp({
    apiKey: config.firebaseApiKey,
    authDomain: config.firebaseAuthDomain,
    projectId: config.firebaseProjectId,
  });
  const auth = getAuth(app);

  if (config.firebaseAuthEmulator) {
    // Point at the local emulator (port 9099 by docker-compose convention).
    connectAuthEmulator(auth, "http://localhost:9099", { disableWarnings: true });
  }

  return {
    provide: {
      firebaseApp: app,
      firebaseAuth: auth,
    },
  };
});

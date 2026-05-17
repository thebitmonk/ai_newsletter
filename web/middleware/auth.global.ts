// Global route guard.
// - Unauthenticated visitors → /login (except public pages).
// - Email/password users whose email is not yet verified → /verify-email.
// - Google users come in already verified, so they bypass the verify step.
// SSR cannot know the auth state, so the guard is a no-op server-side; the
// client-side hydration runs the actual check.

const PUBLIC_PAGES = new Set([
  "/login",
  "/signup",
  "/forgot-password",
  "/verify-email",
]);

export default defineNuxtRouteMiddleware((to) => {
  if (import.meta.server) return;

  const { currentUser } = useFirebaseAuth();
  const path = to.path;

  if (!currentUser.value) {
    if (!PUBLIC_PAGES.has(path)) {
      return navigateTo("/login");
    }
    return;
  }

  // Signed in.
  const providerIds = currentUser.value.providerData.map((p) => p.providerId);
  const usedPassword = providerIds.includes("password");
  if (usedPassword && !currentUser.value.emailVerified) {
    if (path !== "/verify-email") return navigateTo("/verify-email");
    return;
  }

  // Already signed in users shouldn't see login/signup again.
  if (path === "/login" || path === "/signup") {
    return navigateTo("/");
  }
});

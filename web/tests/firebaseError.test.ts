import { describe, expect, it } from "vitest";
import { friendlyFirebaseError } from "../composables/useFirebaseError";

describe("friendlyFirebaseError", () => {
  it("maps known Firebase error codes to friendly text", () => {
    const err = { code: "auth/email-already-in-use", message: "raw" };
    expect(friendlyFirebaseError(err)).toMatch(/already exists/i);
  });

  it("collapses wrong-password and user-not-found to the same friendly message", () => {
    const wrong = { code: "auth/wrong-password", message: "x" };
    const missing = { code: "auth/user-not-found", message: "y" };
    expect(friendlyFirebaseError(wrong)).toBe(friendlyFirebaseError(missing));
  });

  it("falls back to the raw message for unknown Firebase codes", () => {
    const err = { code: "auth/some-future-code", message: "the actual reason" };
    expect(friendlyFirebaseError(err)).toBe("the actual reason");
  });

  it("handles non-Firebase Error objects", () => {
    expect(friendlyFirebaseError(new Error("boom"))).toBe("boom");
  });

  it("stringifies non-Error values", () => {
    expect(friendlyFirebaseError("just a string")).toBe("just a string");
  });
});

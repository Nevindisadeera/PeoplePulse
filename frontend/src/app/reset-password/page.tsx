"use client";

import { Suspense } from "react";
import { SetPassword } from "@/components/set-password";

// The page a person lands on from a password reset link.
export default function ResetPasswordPage() {
  return (
    <Suspense>
      <SetPassword
        endpoint="/auth/reset-password"
        title="Reset your password"
        intro="Choose a new password. You will be signed out on every device and can then sign in with the new one."
        doneText="Your password has been changed. Sign in with the new password."
      />
    </Suspense>
  );
}

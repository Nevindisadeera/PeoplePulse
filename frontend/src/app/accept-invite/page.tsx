"use client";

import { Suspense } from "react";
import { SetPassword } from "@/components/set-password";

// The page an invited person lands on from their invitation link.
export default function AcceptInvitePage() {
  // The form reads the token from the address, which needs a Suspense boundary to prerender.
  return (
    <Suspense>
      <SetPassword
        endpoint="/auth/accept-invite"
        title="Choose a password"
        intro="You have been invited to PeoplePulse. Set a password to activate your account."
        doneText="Your password is saved. Sign in with your email address and the password you just chose."
      />
    </Suspense>
  );
}

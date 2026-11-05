import { useMutation } from "@tanstack/react-query";
import { toast } from "sonner";

import {
  captureProfileRequestContext,
  isCapturedProfileAuthorityActive,
  StaleApiRequestContextError,
} from "@/api/client";
import { v2 } from "@/api/v2/request";
import type { components } from "@/api/v2/schema";

export type SeasonalBackground = components["schemas"]["SeasonalBackground"];

/**
 * Uploads a Seasonal Picks background image (multipart) and answers its
 * content-addressed reference and URL. Nothing shows the image until a row
 * saves the reference, so this mutation invalidates nothing: the editor holds
 * the reference in the draft it is editing.
 *
 * Non-retryable like the branding uploads: the server stores a new object
 * unconditionally, so a delayed replay would leave a second orphaned image
 * rather than replace one. The authority is captured at click time, and a
 * changed authority refuses to run.
 */
export function useUploadSeasonalBackground() {
  return useMutation({
    retry: false,
    mutationFn: async (file: File) => {
      const authority = captureProfileRequestContext();
      if (authority === null || !isCapturedProfileAuthorityActive(authority)) {
        throw new StaleApiRequestContextError();
      }
      return v2("POST /api/v2/admin/sections/seasonal-backgrounds", {
        form: { file },
        profileContext: authority,
        retryAuthentication: false,
      });
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Failed to upload the background");
    },
  });
}

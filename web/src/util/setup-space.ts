import type { SetupAssessment } from "../api/setup-assessment-types";

export function setupRequiredBytes(assessment: SetupAssessment, localInstall: boolean, voice: string, input: boolean): number {
  const selectedVoice = assessment.voice_options?.find(option => option.module === voice)?.requirement ?? assessment.voice_output;
  return (localInstall && assessment.chat.status !== "unmet" ? assessment.chat.bytes : 0)
    + (voice !== "none" && selectedVoice.status !== "unmet" ? selectedVoice.bytes : 0)
    + (input && assessment.voice_input.status !== "unmet" ? assessment.voice_input.bytes : 0);
}

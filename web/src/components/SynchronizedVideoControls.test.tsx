import { act, fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { SynchronizedVideoControls } from "./SynchronizedVideoControls";

function renderControls(overrides: Partial<Parameters<typeof SynchronizedVideoControls>[0]> = {}) {
  const props: Parameters<typeof SynchronizedVideoControls>[0] = {
    autoHide: false,
    currentTimeMillis: 1_000,
    durationMillis: 10_000,
    muted: false,
    playbackIntent: true,
    playbackRate: 1,
    volume: 0.8,
    onFullscreen: vi.fn(),
    onMuteChange: vi.fn(),
    onPlaybackRateChange: vi.fn(),
    onSeekCancel: vi.fn(),
    onSeekCommit: vi.fn(),
    onSeekStart: vi.fn(),
    onTogglePlayback: vi.fn(),
    onVolumeChange: vi.fn(),
    ...overrides,
  };
  render(<SynchronizedVideoControls {...props} />);
  return props;
}

describe("SynchronizedVideoControls", () => {
  it("captures one pointer scrub and commits its final timestamp", () => {
    const props = renderControls();
    const slider = screen.getByRole("slider", { name: "Video position" }) as HTMLInputElement;
    const setPointerCapture = vi.fn();
    const releasePointerCapture = vi.fn();
    Object.assign(slider, {
      setPointerCapture,
      hasPointerCapture: () => true,
      releasePointerCapture,
    });

    fireEvent.pointerDown(slider, { pointerId: 7 });
    fireEvent.change(slider, { target: { value: "4500" } });
    fireEvent.pointerUp(slider, { pointerId: 7 });
    fireEvent.lostPointerCapture(slider, { pointerId: 7 });

    expect(props.onSeekStart).toHaveBeenCalledOnce();
    expect(props.onSeekCommit).toHaveBeenCalledWith(4_500);
    expect(props.onSeekCancel).not.toHaveBeenCalled();
    expect(setPointerCapture).toHaveBeenCalledOnce();
    expect(releasePointerCapture).toHaveBeenCalledOnce();
  });

  it("cancels a captured scrub without committing it", () => {
    const props = renderControls();
    const slider = screen.getByRole("slider", { name: "Video position" }) as HTMLInputElement;
    Object.assign(slider, {
      setPointerCapture: vi.fn(),
      hasPointerCapture: () => false,
    });

    fireEvent.pointerDown(slider, { pointerId: 9 });
    fireEvent.change(slider, { target: { value: "6500" } });
    fireEvent.pointerCancel(slider, { pointerId: 9 });

    expect(props.onSeekStart).toHaveBeenCalledOnce();
    expect(props.onSeekCancel).toHaveBeenCalledOnce();
    expect(props.onSeekCommit).not.toHaveBeenCalled();
  });

  it("commits keyboard seek intent on key release", () => {
    const props = renderControls();
    const slider = screen.getByRole("slider", { name: "Video position" });

    fireEvent.change(slider, { target: { value: "2500" } });
    fireEvent.keyUp(slider, { key: "ArrowRight" });

    expect(props.onSeekStart).toHaveBeenCalledOnce();
    expect(props.onSeekCommit).toHaveBeenCalledWith(2_500);
  });

  it("keeps volume adjustment anchored to the mute control", () => {
    const props = renderControls();
    const mute = screen.getByRole("button", { name: "Mute video" });
    const volume = screen.getByRole("slider", { name: "Video volume" });

    expect(mute.closest(".media-transport-volume-control")).toContainElement(volume);
    fireEvent.change(volume, { target: { value: "0.35" } });
    expect(props.onVolumeChange).toHaveBeenCalledWith(0.35);
  });

  it("hides inactive playback controls and reveals them for pointer or keyboard use", () => {
    vi.useFakeTimers();
    try {
      renderControls({ autoHide: true });
      const group = screen.getByRole("group", { name: "Synchronized video controls" });
      const overlay = group.parentElement;
      if (!overlay) throw new Error("transport overlay missing");

      expect(overlay).toHaveAttribute("data-visible", "true");
      act(() => vi.advanceTimersByTime(2_000));
      expect(overlay).toHaveAttribute("data-visible", "false");

      fireEvent.pointerMove(overlay);
      expect(overlay).toHaveAttribute("data-visible", "true");

      const play = screen.getByRole("button", { name: "Pause video and motion" });
      fireEvent.focus(play);
      act(() => vi.advanceTimersByTime(2_000));
      expect(overlay).toHaveAttribute("data-visible", "true");

      fireEvent.blur(play, { relatedTarget: null });
      act(() => vi.advanceTimersByTime(2_000));
      expect(overlay).toHaveAttribute("data-visible", "false");
    } finally {
      vi.useRealTimers();
    }
  });
});

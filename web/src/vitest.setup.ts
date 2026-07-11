import "@testing-library/jest-dom/vitest";

class TestResizeObserver {
  observe() {}
  disconnect() {}
}

Object.defineProperty(globalThis, "ResizeObserver", { value: TestResizeObserver, configurable: true });
Object.defineProperty(HTMLCanvasElement.prototype, "getContext", {
  configurable: true,
  value: () => ({
    scale() {}, fillRect() {}, beginPath() {}, moveTo() {}, lineTo() {}, stroke() {}, arc() {}, fill() {},
    fillStyle: "", strokeStyle: "", lineWidth: 1,
  }),
});

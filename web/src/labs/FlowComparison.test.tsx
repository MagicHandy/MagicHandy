import {fireEvent,render,screen} from "@testing-library/react";
import {describe,expect,it} from "vitest";
import {initialFlow} from "./api";
import {labPreview} from "./fixtures";
import {FlowComparison} from "./FlowComparison";

function flowPreview() {
  const preview=labPreview(initialFlow);
  const flow=preview.candidates.find(candidate=>candidate.method==="flow")!;
  flow.samples=[{time_ms:0,position_percent:0,velocity_percent_per_second:0},{time_ms:6000,position_percent:100,velocity_percent_per_second:120.5},{time_ms:12000,position_percent:50,velocity_percent_per_second:-80}];
  return {preview,flow};
}

describe("Flow comparison charts",()=>{
  it("draws backend samples edge to edge with their scales as HTML outside the drawing",()=>{
    const {preview,flow}=flowPreview();
    const {container}=render(<FlowComparison preview={preview} selected={flow} compact/>);
    const chart=screen.getByRole("img",{name:"Planned position comparison"});
    expect(chart).toHaveAttribute("viewBox","0 0 700 200");
    expect(chart).toHaveAttribute("preserveAspectRatio","none");
    expect(chart.querySelector(".motion-lab-selected")).toHaveAttribute("points","0,200 350,0 700,100");
    expect(container.querySelector("svg text")).toBeNull();
    const plot=chart.closest(".pattern-plot");
    expect(plot?.querySelector(".pattern-plot-y")).toHaveTextContent("Tip 10050Base 0");
    expect(plot?.querySelector(".pattern-plot-x")).toHaveTextContent("0 s6 s12 s");
  });
  it("scales the velocity estimate by its peak in both directions",()=>{
    const {preview,flow}=flowPreview();
    render(<FlowComparison preview={preview} selected={flow}/>);
    fireEvent.click(screen.getByText("Compare methods and dynamics"));
    const chart=screen.getByRole("img",{name:"Velocity estimate"});
    expect(chart.querySelector(".motion-lab-selected")).toHaveAttribute("points",`0,100 350,0 700,${100-(-80)/120.5*100}`);
    const plot=chart.closest(".pattern-plot");
    expect(plot?.querySelector(".pattern-plot-y")).toHaveTextContent("120.5 %/s0-120.5 %/s");
    expect(plot?.querySelector(".pattern-plot-x")).toHaveTextContent("0 s6 s12 s");
  });
});

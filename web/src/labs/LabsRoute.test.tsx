import {render,screen} from "@testing-library/react";
import {beforeEach,describe,expect,it,vi} from "vitest";
import {LabsRoute} from "./LabsRoute";
import {LabsNavLink,legacyLabRoute} from "./enabled";
import {routeBase} from "../shell/NavRail";

const route=vi.hoisted(()=>({hash:"#/labs/chat"}));
vi.mock("../state/app-state",()=>({useHashRoute:()=>route.hash}));
vi.mock("./LLMLab",()=>({LLMLab:()=> <div>Chat workspace</div>}));
vi.mock("../components/MotionLab",()=>({MotionLab:()=> <div>Motion workspace</div>}));
vi.mock("./Observations",()=>({ObservationsPage:()=> <div>Observation workspace</div>}));
vi.mock("./TestRuns",()=>({TestRuns:()=> <div>Guided testing workspace</div>}));
vi.mock("../components/WorkspaceHead",()=>({WorkspaceHead:()=> <h1>Labs</h1>}));
describe("development Labs navigation",()=>{
  beforeEach(()=>{route.hash="#/labs/chat";});
  it("has four page links and renders only the selected workspace",()=>{
    route.hash="#/labs/motion";render(<LabsRoute/>);
    expect(screen.getByRole("link",{name:"Motion Lab"})).toHaveAttribute("aria-current","page");
    expect(screen.getAllByRole("link")).toHaveLength(4);
    expect(screen.getByText("Motion workspace")).toBeInTheDocument();
    expect(screen.queryByText("Chat workspace")).not.toBeInTheDocument();
  });
  it("requires backend Labs enablement for its sidebar entry",()=>{
    const view=render(<LabsNavLink active="labs" enabled={false}/>);
    expect(screen.queryByRole("link",{name:"Labs"})).not.toBeInTheDocument();
    view.rerender(<LabsNavLink active="labs" enabled/>);
    expect(screen.getByRole("link",{name:"Labs"})).toHaveAttribute("aria-current","page");
    expect(routeBase("#/labs/observations")).toBe("labs");
  });
  it("retains visited chat and motion editors while only showing the active tab",()=>{
    const view=render(<LabsRoute/>);
    expect(screen.getByText("Chat workspace")).toBeVisible();
    route.hash="#/labs/motion";view.rerender(<LabsRoute/>);
    expect(screen.getByText("Chat workspace")).not.toBeVisible();
    expect(screen.getByText("Motion workspace")).toBeVisible();
    route.hash="#/labs/chat";view.rerender(<LabsRoute/>);
    expect(screen.getByText("Chat workspace")).toBeVisible();
    expect(screen.getByText("Motion workspace")).not.toBeVisible();
  });
  it("maps the previous Settings bookmarks to the new pages",()=>{
    expect(legacyLabRoute("#/settings/motion-lab")).toBe("#/labs/motion");
    expect(legacyLabRoute("#/settings/llm-lab")).toBe("#/labs/chat");
    expect(legacyLabRoute("#/settings/motion")).toBe("");
  });
});

import {render,screen} from "@testing-library/react";
import {describe,expect,it,vi} from "vitest";
import {LabsRoute} from "./enabled";
vi.mock("../state/app-state",()=>({useAppState:()=>({state:{labs_enabled:false}})}));
vi.mock("../components/WorkspaceHead",()=>({WorkspaceHead:()=> <h1>Labs</h1>}));
vi.mock("./LabsRoute",()=>{throw new Error("disabled Labs must not load the workspace");});
describe("disabled Labs bookmark",()=>{
  it("offers the settings route without loading lab tools",()=>{
    render(<LabsRoute/>);
    expect(screen.getByText("Labs is disabled. Enable it in Settings > General.")).toBeInTheDocument();
    expect(screen.getByRole("link",{name:"Open Settings"})).toHaveAttribute("href","#/settings/general");
  });
});

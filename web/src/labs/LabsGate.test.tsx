import {render,screen} from "@testing-library/react";
import {describe,expect,it,vi} from "vitest";
import {LabsRoute} from "./enabled";
const app=vi.hoisted(()=>({hostAdministration:true,labsEnabled:false}));
vi.mock("../state/app-state",()=>({useAppState:()=>({state:{labs_enabled:app.labsEnabled,capabilities:{configure_host:app.hostAdministration}}})}));
vi.mock("../components/WorkspaceHead",()=>({WorkspaceHead:()=> <h1>Labs</h1>}));
vi.mock("./LabsRoute",()=>{throw new Error("disabled Labs must not load the workspace");});
describe("disabled Labs bookmark",()=>{
  it("offers the settings route without loading lab tools",()=>{
    render(<LabsRoute/>);
    expect(screen.getByText("Labs is disabled. Enable it in Settings > General.")).toBeInTheDocument();
    expect(screen.getByRole("link",{name:"Open Settings"})).toHaveAttribute("href","#/settings/general");
  });
  it("keeps enabled host labs out of an observer bookmark",()=>{
    app.hostAdministration=false;
    app.labsEnabled=true;
    render(<LabsRoute/>);
    expect(screen.getByText("Host settings and diagnostics are managed by an administrator.")).toBeInTheDocument();
    expect(screen.queryByRole("link",{name:"Open Settings"})).not.toBeInTheDocument();
  });
});

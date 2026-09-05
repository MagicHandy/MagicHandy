import {fireEvent,render,screen,waitFor} from "@testing-library/react";
import {beforeEach,describe,expect,it,vi} from "vitest";
import {api} from "../api/client";
import {LabsSettings} from "./LabsSettings";

const app=vi.hoisted(()=>({state:{labs_enabled:false},backendOnline:true,readOnly:false,refresh:vi.fn()}));
vi.mock("../state/app-state",()=>({useAppState:()=>app}));
vi.mock("../api/client",()=>({api:{setLabsEnabled:vi.fn()}}));
describe("release Labs setting",()=>{
  beforeEach(()=>{vi.clearAllMocks();app.state.labs_enabled=false;app.backendOnline=true;app.readOnly=false;app.refresh.mockResolvedValue(undefined);vi.mocked(api.setLabsEnabled).mockResolvedValue({enabled:true});});
  it("saves immediately and displays only the authoritative enabled state",async()=>{
    const view=render(<LabsSettings/>);
    fireEvent.click(screen.getByRole("checkbox",{name:"Enable Labs"}));
    await waitFor(()=>expect(app.refresh).toHaveBeenCalledOnce());
    expect(api.setLabsEnabled).toHaveBeenCalledWith(true);
    expect(screen.getByRole("checkbox")).not.toBeChecked();
    app.state.labs_enabled=true;view.rerender(<LabsSettings/>);
    expect(screen.getByRole("checkbox")).toBeChecked();expect(screen.getByRole("link",{name:"Open Labs"})).toHaveAttribute("href","#/labs/chat");
  });
  it("keeps the checkbox unchanged and surfaces a failed save",async()=>{
    vi.mocked(api.setLabsEnabled).mockRejectedValue(new Error("Labs setting could not be saved"));
    render(<LabsSettings/>);fireEvent.click(screen.getByRole("checkbox"));
    await screen.findByRole("alert");expect(screen.getByRole("checkbox")).not.toBeChecked();
  });
  it("disables mutations for readers and backend-offline clients",()=>{
    app.readOnly=true;const view=render(<LabsSettings/>);expect(screen.getByRole("checkbox")).toBeDisabled();
    app.readOnly=false;app.backendOnline=false;view.rerender(<LabsSettings/>);expect(screen.getByRole("checkbox")).toBeDisabled();
  });
});

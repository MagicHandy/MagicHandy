import {act,fireEvent,render,screen,waitFor} from "@testing-library/react";
import {beforeEach,describe,expect,it,vi} from "vitest";
import {api} from "../api/client";
import {labApi,type FlowPreview} from "../labs/api";
import {labLimits,labPreview} from "../labs/fixtures";
import {MotionLab} from "./MotionLab";

const app=vi.hoisted(()=>({backendOnline:true,readOnly:false,state:{motion_simulated:false,settings:{motion:{}}},refresh:vi.fn(),show:vi.fn()}));
vi.mock("../state/app-state",()=>({useAppState:()=>app,useToast:()=>({show:app.show})}));
vi.mock("../api/client",()=>({api:{stopMotion:vi.fn()}}));
vi.mock("../labs/api",async importOriginal=>({...await importOriginal<typeof import("../labs/api")>(),labApi:{preview:vi.fn(),start:vi.fn()}}));

describe("Motion Lab flow comparisons",()=>{
  beforeEach(()=>{vi.clearAllMocks();app.backendOnline=true;app.readOnly=false;app.state.motion_simulated=false;app.state.settings.motion=labLimits;
    vi.mocked(labApi.preview).mockImplementation(async spec=>labPreview(spec));vi.mocked(labApi.start).mockResolvedValue({});vi.mocked(api.stopMotion).mockResolvedValue({});});
  it("starts only the explicitly selected backend preview",async()=>{
    render(<MotionLab/>);const start=await screen.findByRole("button",{name:"Start selected test"});
    expect(labApi.start).not.toHaveBeenCalled();
    fireEvent.change(screen.getByRole("combobox",{name:"Test generator"}),{target:{value:"anchored"}});fireEvent.click(start);
    await waitFor(()=>expect(labApi.start).toHaveBeenCalledOnce());
    expect(vi.mocked(labApi.start).mock.calls[0][1].method).toBe("anchored");
  });
  it("separates score examples from generator selection and hides inapplicable controls",async()=>{
    render(<MotionLab/>);await screen.findByRole("button",{name:"Start selected test"});
    fireEvent.click(screen.getByRole("button",{name:"Layered score"}));
    await screen.findByText("Test target: Continuous flow · Layered score");
    expect(screen.getByRole("combobox",{name:"Test generator"})).toHaveValue("flow");
    fireEvent.change(screen.getByRole("combobox",{name:"Test generator"}),{target:{value:"creative"}});
    expect(screen.getByText("Test target: Creative baseline")).toBeInTheDocument();
    expect(screen.queryByRole("button",{name:"Layered score"})).not.toBeInTheDocument();
    expect(screen.queryByRole("slider",{name:/Range memory/})).not.toBeInTheDocument();
    expect(screen.getByRole("slider",{name:/Range anchor/})).toBeDisabled();
    expect(screen.getByRole("slider",{name:/Range anchor/})).toHaveValue("50");
    fireEvent.click(screen.getByText("Compare methods and dynamics"));
    expect(screen.queryByRole("button",{name:"Anchored range"})).not.toBeInTheDocument();
    fireEvent.change(screen.getByRole("combobox",{name:"Test generator"}),{target:{value:"flow"}});
    expect(screen.getByText("Test target: Continuous flow · Layered score")).toBeInTheDocument();
    expect(labApi.start).not.toHaveBeenCalled();
  });
  it("keeps Stop available for read-only clients",async()=>{
    app.readOnly=true;render(<MotionLab/>);expect(await screen.findByRole("button",{name:"Start selected test"})).toBeDisabled();
    fireEvent.click(screen.getByRole("button",{name:"Stop"}));expect(api.stopMotion).toHaveBeenCalledOnce();
  });
  it("compiles experimental controls without starting motion and keeps them out of historical references",async()=>{
    render(<MotionLab/>);await screen.findByRole("button",{name:"Start selected test"});
    fireEvent.change(screen.getByRole("combobox",{name:"Variation source"}),{target:{value:"drift"}});
    fireEvent.change(screen.getByRole("slider",{name:/Turn softness/}),{target:{value:"70"}});
    fireEvent.change(screen.getByRole("slider",{name:/Steady beat/}),{target:{value:"100"}});
    await waitFor(()=>expect(labApi.preview).toHaveBeenLastCalledWith(expect.objectContaining({variation_mode:"drift",turn_softness_percent:70,cadence_hold_percent:100}),expect.any(AbortSignal)));
    expect(labApi.start).not.toHaveBeenCalled();
    fireEvent.change(screen.getByRole("combobox",{name:"Test generator"}),{target:{value:"anchored"}});
    expect(screen.queryByRole("combobox",{name:"Variation source"})).not.toBeInTheDocument();
    expect(screen.queryByRole("slider",{name:/Turn softness/})).not.toBeInTheDocument();
    expect(screen.queryByRole("button",{name:"Compare motion experiments"})).not.toBeInTheDocument();
  });
  it("labels simulator routing before an audition",async()=>{
    app.state.motion_simulated=true;render(<MotionLab/>);expect(await screen.findByRole("button",{name:"Start simulated test"})).toBeEnabled();
    expect(screen.getByText(/Simulation is active/)).toBeInTheDocument();
  });
  it("discards a late preview after editing",async()=>{
    let resolveOld:((result:FlowPreview)=>void)|undefined;
    vi.mocked(labApi.preview).mockImplementationOnce(()=>new Promise(resolve=>{resolveOld=resolve;}));
    render(<MotionLab/>);await waitFor(()=>expect(labApi.preview).toHaveBeenCalledOnce());const old=vi.mocked(labApi.preview).mock.calls[0][0];
    fireEvent.change(screen.getByRole("slider",{name:/^Speed/}),{target:{value:"35"}});
    await screen.findByRole("button",{name:"Start selected test"});await act(async()=>resolveOld?.(labPreview(old)));
    fireEvent.click(screen.getByRole("button",{name:"Start selected test"}));await waitFor(()=>expect(labApi.start).toHaveBeenCalledOnce());
    expect(vi.mocked(labApi.start).mock.calls[0][0].spec.speed_percent).toBe(35);
  });
});

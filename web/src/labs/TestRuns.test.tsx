import {fireEvent,render,screen,waitFor} from "@testing-library/react";
import {beforeEach,describe,expect,it,vi} from "vitest";
import {api} from "../api/client";
import {initialFlow,labApi} from "./api";
import {labLimits,labPreview} from "./fixtures";
import {TestRuns} from "./TestRuns";
import {CreateTestSequence} from "./CreateTestSequence";
import {testApi,type TestRunView} from "./test-api";

const app=vi.hoisted(()=>({hash:"#/labs/tests/run-1",state:{settings:{motion:{}},motion_simulated:false},motion:{available:true,engine:{running:false,paused:false}},backendOnline:true,readOnly:false,refresh:vi.fn()}));
vi.mock("../state/app-state",()=>({useAppState:()=>app,useHashRoute:()=>app.hash}));
vi.mock("../api/client",()=>({api:{stopMotion:vi.fn()}}));
vi.mock("./api",async original=>({...await original<typeof import("./api")>(),labApi:{start:vi.fn()}}));
vi.mock("./test-api",async original=>({...await original<typeof import("./test-api")>(),testApi:{list:vi.fn(),get:vi.fn(),create:vi.fn(),feedback:vi.fn(),remove:vi.fn()}}));

function view():TestRunView {
  return {next_index:0,can_audition:true,storage_path:"C:/review/magichandy.db",run:{id:"run-1",title:"Motion feel check",created_at:"2026-09-04T00:00:00Z",version:"dev",commit:"test",revision:1,
    steps:["First test","Second test"].map((title,index)=>({id:`step-${index}`,title,instruction:"Look for smooth reversals.",source:{id:`source-${index}`,created_at:"2026-09-04T00:00:00Z",text:"",source:"motion",label:title,method:"flow",spec:initialFlow,settings:labLimits},preview:{...labPreview(initialFlow),candidates:labPreview(initialFlow).candidates.filter(candidate=>candidate.flow)}}))}};
}
function answer() {
  fireEvent.click(screen.getByRole("radio",{name:"Mixed"}));
  fireEvent.change(screen.getByRole("combobox",{name:"What did you review?"}),{target:{value:"preview"}});
  fireEvent.change(screen.getByRole("textbox",{name:"Comment for this test"}),{target:{value:"The reversal looks abrupt."}});
}

describe("guided Lab test sequences",()=>{
  beforeEach(()=>{vi.clearAllMocks();app.hash="#/labs/tests/run-1";app.readOnly=false;app.backendOnline=true;app.motion.engine.running=false;app.motion.available=true;app.state.settings.motion=labLimits;
    vi.mocked(testApi.get).mockResolvedValue(view());vi.mocked(testApi.list).mockResolvedValue({runs:[],capacity:20,storage_path:"C:/review/magichandy.db"});vi.mocked(labApi.start).mockResolvedValue({});vi.mocked(api.stopMotion).mockResolvedValue({});});
  it("shows one round, requires a rating and basis, then saves the exact feedback without starting motion",async()=>{
    const next=view();next.next_index=1;next.run.revision=2;next.run.steps[0].feedback={rating:2,basis:"preview",comment:"The reversal looks abrupt.",created_at:"2026-09-04T00:00:00Z"};
    vi.mocked(testApi.feedback).mockResolvedValue(next);
    render(<TestRuns/>);await screen.findByRole("heading",{name:"First test"});
    expect(screen.queryByRole("heading",{name:"Second test"})).not.toBeInTheDocument();
    expect(screen.getByRole("button",{name:"Save and next"})).toBeDisabled();answer();fireEvent.click(screen.getByRole("button",{name:"Save and next"}));
    await screen.findByRole("heading",{name:"Second test"});
    expect(testApi.feedback).toHaveBeenCalledWith(expect.objectContaining({next_index:0}),2,"preview","The reversal looks abrupt.");
    expect(screen.getByRole("progressbar")).toHaveAttribute("value","1");
    expect(screen.getByRole("textbox",{name:"Comment for this test"})).toHaveValue("");
    expect(labApi.start).not.toHaveBeenCalled();
  });
  it("retains the user's answer when saving fails",async()=>{
    vi.mocked(testApi.feedback).mockRejectedValue(new Error("Storage unavailable"));
    render(<TestRuns/>);await screen.findByRole("heading",{name:"First test"});answer();fireEvent.click(screen.getByRole("button",{name:"Save and next"}));
    await screen.findByRole("alert");expect(screen.getByRole("textbox",{name:"Comment for this test"})).toHaveValue("The reversal looks abrupt.");
    expect(screen.getByRole("radio",{name:"Mixed"})).toBeChecked();expect(screen.getByRole("progressbar")).toHaveAttribute("value","0");
  });
  it("preserves comments on skipped rounds and resumes saved progress",async()=>{
    const current=view();current.next_index=1;current.run.revision=2;current.run.steps[0].feedback={rating:1,basis:"preview",comment:"Needs a smoother turn.",created_at:"2026-09-04T00:00:00Z"};
    const complete=structuredClone(current);complete.next_index=2;complete.run.revision=3;complete.can_audition=false;complete.run.steps[1].feedback={rating:0,basis:"skipped",comment:"Device unavailable.",created_at:"2026-09-04T00:00:00Z"};
    vi.mocked(testApi.get).mockResolvedValue(current);vi.mocked(testApi.feedback).mockResolvedValue(complete);
    render(<TestRuns/>);await screen.findByRole("heading",{name:"Second test"});fireEvent.change(screen.getByRole("textbox",{name:"Comment for this test"}),{target:{value:"Device unavailable."}});fireEvent.click(screen.getByRole("button",{name:"Skip this round"}));
    await screen.findByText("Feedback collected");expect(testApi.feedback).toHaveBeenCalledWith(current,0,"skipped","Device unavailable.");expect(screen.getByText("1 reviewed · 1 skipped")).toBeInTheDocument();expect(screen.getByText("Device unavailable.")).toBeInTheDocument();expect(labApi.start).not.toHaveBeenCalled();
  });
  it("keeps Stop usable and prevents moving or read-only clients from advancing",async()=>{
    app.motion.engine.running=true;render(<TestRuns/>);await screen.findByRole("heading",{name:"First test"});answer();expect(screen.getByRole("button",{name:"Save and next"})).toBeDisabled();expect(screen.getByRole("button",{name:"Skip this round"})).toBeDisabled();fireEvent.click(screen.getByRole("button",{name:"Stop"}));await waitFor(()=>expect(api.stopMotion).toHaveBeenCalled());
  });
  it("blocks auditions for changed captures while allowing visual feedback",async()=>{
    const changed=view();changed.can_audition=false;changed.warning="Saved limits changed.";vi.mocked(testApi.get).mockResolvedValue(changed);
    render(<TestRuns/>);await screen.findByRole("heading",{name:"First test"});expect(screen.getByRole("button",{name:"Audition this round"})).toBeDisabled();answer();expect(screen.getByRole("button",{name:"Save and next"})).toBeEnabled();
  });
  it("uses the existing audition path only on an explicit click",async()=>{
    render(<TestRuns/>);await screen.findByRole("heading",{name:"First test"});expect(labApi.start).not.toHaveBeenCalled();fireEvent.click(screen.getByRole("button",{name:"Audition this round"}));await waitFor(()=>expect(labApi.start).toHaveBeenCalledWith(view().run.steps[0].preview,view().run.steps[0].preview!.candidates[0]));
  });
  it("creates a guided comparison from the exact selected LLM reply",async()=>{
    const target={source:"llm" as const,settings_key:"saved",revision:4,turn_index:2};vi.mocked(testApi.create).mockResolvedValue(view());
    render(<CreateTestSequence target={target}/>);fireEvent.click(screen.getByRole("button",{name:"Create test sequence"}));await waitFor(()=>expect(testApi.create).toHaveBeenCalledWith("llm_comparison",target));expect(labApi.start).not.toHaveBeenCalled();
  });
  it("offers a starter sequence and stored runs without optimistic progress",async()=>{
    app.hash="#/labs/tests";app.readOnly=true;render(<TestRuns/>);await screen.findByText("Your first sequence will appear here. Saved progress survives restarts.");expect(screen.getByRole("button",{name:"Start a motion feel check"})).toBeDisabled();expect(testApi.create).not.toHaveBeenCalled();
  });
  it("captures the selected flow for the five-round experiment comparison without auditioning it",async()=>{
    const target={source:"motion" as const,method:"flow",spec:{...initialFlow,anchor_percent:100},settings_key:"saved"};
    vi.mocked(testApi.create).mockResolvedValue(view());render(<CreateTestSequence experiments target={target}/>);
    fireEvent.click(screen.getByRole("button",{name:"Compare motion experiments"}));
    await waitFor(()=>expect(testApi.create).toHaveBeenCalledWith("motion_experiments",target));expect(labApi.start).not.toHaveBeenCalled();
  });
  it("keeps the changed reply out of the before round",async()=>{
    const before=view();before.run.steps[0].phase="before";before.run.steps[0].source.source="llm";
    before.run.steps[0].source.trial={message:"Hold the tip",reply:"Tip anchor applied.",raw:"{}",valid:true,changed:["anchor_percent"],method:"controls",model:"local",prompt:"test",elapsed_ms:100,provider_calls:1,before:initialFlow,after:{...initialFlow,anchor_percent:100}};
    vi.mocked(testApi.get).mockResolvedValue(before);render(<TestRuns/>);await screen.findByRole("heading",{name:"First test"});
    expect(screen.queryByRole("option",{name:"LLM response"})).not.toBeInTheDocument();expect(screen.queryByText("Tip anchor applied.")).not.toBeInTheDocument();
    expect(screen.getByText("Hold the tip")).toBeInTheDocument();
  });
});

import {fireEvent,render,screen,waitFor} from "@testing-library/react";
import {beforeEach,describe,expect,it,vi} from "vitest";
import {api} from "../api/client";
import {labApi,type LabCompareResult,type LabTrial} from "./api";
import {labLimits,labPreview,labState} from "./fixtures";
import {LLMLab} from "./LLMLab";

const app=vi.hoisted(()=>({state:{settings:{motion:{}}},backendOnline:true,readOnly:false,refresh:vi.fn(),show:vi.fn()}));
vi.mock("../state/app-state",()=>({useMotionState: () => null,
  useAppState:()=>app,useToast:()=>({show:app.show})}));
vi.mock("../api/client",()=>({api:{stopMotion:vi.fn()}}));
vi.mock("./api",async importOriginal=>({...await importOriginal<typeof import("./api")>(),labApi:{state:vi.fn(),status:vi.fn(),preview:vi.fn(),chat:vi.fn(),reset:vi.fn(),start:vi.fn(),session:vi.fn(),compare:vi.fn()}}));

const empty="Pick a mode, then type a request or tap a quick one.";
function trial(message:string,changes:Partial<LabTrial>={}):LabTrial {
  return {message,reply:"Reply.",raw:"{}",valid:true,changed:["anchor_percent"],model:"local-model",method:"layered",prompt:"layered-prompt",
    elapsed_ms:500,provider_calls:1,before:labState().current,after:labState().current,...changes};
}
async function ready() {render(<LLMLab/>);await screen.findByText(empty);}
function type(text:string) {fireEvent.change(screen.getByRole("textbox",{name:"Message"}),{target:{value:text}});}

describe("streamlined LLM Lab",()=>{
  beforeEach(()=>{vi.clearAllMocks();app.state.settings.motion=labLimits;app.readOnly=false;
    vi.mocked(labApi.status).mockResolvedValue({revision:0,busy:false});
    vi.mocked(labApi.state).mockResolvedValue(labState());vi.mocked(labApi.preview).mockImplementation(async spec=>labPreview(spec));vi.mocked(labApi.start).mockResolvedValue({});});
  it("keeps chat as preview until Live motion is switched on",async()=>{
    const next=labState();next.revision=1;next.current.anchor_percent=100;
    next.turns=[trial("Hold the tip",{reply:"Tip anchor in preview.",method:"controls",prompt:"control-prompt",after:next.current})];
    vi.mocked(labApi.chat).mockResolvedValue(next);
    await ready();
    type("Hold the tip");fireEvent.click(screen.getByRole("button",{name:"Send"}));
    await screen.findByText("Tip anchor in preview.");expect(labApi.start).not.toHaveBeenCalled();
    vi.mocked(labApi.session).mockResolvedValue({...next,session:{active:true,live:true,autopilot:false,method:"layered",prompt:"layered-prompt",model:"local-model",schema_guided:true,interval_seconds:20}});
    fireEvent.click(screen.getByRole("switch",{name:"Live motion"}));
    await screen.findByText("Live test running");
    expect(labApi.session).toHaveBeenCalledWith(expect.objectContaining({live:true,autopilot:false,method:"layered"}));
    expect(api.stopMotion).not.toHaveBeenCalled();
    expect(vi.mocked(labApi.chat).mock.calls[0][0]).toMatchObject({schema_guided:true,revision:0,method:"layered"});
  });
  it("switching Live motion off ends the running test",async()=>{
    const running={...labState(),session:{active:true,live:true,autopilot:false,method:"layered",prompt:"layered-prompt",model:"local-model",schema_guided:true,interval_seconds:20}};
    vi.mocked(labApi.state).mockResolvedValueOnce(running).mockResolvedValue(labState());
    await ready();
    fireEvent.click(await screen.findByRole("switch",{name:"Live motion",checked:true}));
    await waitFor(()=>expect(api.stopMotion).toHaveBeenCalledOnce());
    expect(labApi.session).not.toHaveBeenCalled();
  });
  it("keeps generation disabled for read-only clients",async()=>{
    app.readOnly=true;await ready();
    expect(screen.getByRole("button",{name:"Send"})).toBeDisabled();expect(screen.getByRole("switch",{name:"Live motion"})).toBeDisabled();
    expect(screen.getByRole("button",{name:"Deeper"})).toBeDisabled();
  });
  it("loads an authoritative new score when selecting Creative v2",async()=>{
    vi.mocked(labApi.reset).mockResolvedValue({...labState(),revision:1});
    vi.mocked(labApi.chat).mockResolvedValue({...labState(),revision:2});
    await ready();
    fireEvent.click(screen.getByRole("button",{name:"Creative v2"}));
    await waitFor(()=>expect(labApi.reset).toHaveBeenCalledWith(undefined,"creative_v2"));
    await waitFor(()=>expect(screen.getByRole("button",{name:"Creative v2"})).toHaveAttribute("aria-pressed","true"));
    type("Mix full strokes with base rebounds.");fireEvent.click(screen.getByRole("button",{name:"Send"}));
    await waitFor(()=>expect(labApi.chat).toHaveBeenCalledWith(expect.objectContaining({method:"creative_v2",prompt:"gesture-prompt",revision:1}),expect.any(AbortSignal)));
  });
  it("loads a stroke mode's own score and prompt",async()=>{
    vi.mocked(labApi.reset).mockResolvedValue({...labState(),revision:1});
    vi.mocked(labApi.chat).mockResolvedValue({...labState(),revision:2});
    await ready();
    fireEvent.click(screen.getByRole("button",{name:"Plain words"}));
    await waitFor(()=>expect(labApi.reset).toHaveBeenCalledWith(undefined,"plain_words"));
    await waitFor(()=>expect(screen.getByRole("button",{name:"Plain words"})).toHaveAttribute("aria-pressed","true"));
    type("Only the head.");fireEvent.click(screen.getByRole("button",{name:"Send"}));
    await waitFor(()=>expect(labApi.chat).toHaveBeenCalledWith(expect.objectContaining({method:"plain_words",prompt:"plain-prompt"}),expect.any(AbortSignal)));
  });
  it("sends the selected library naming contract with its matching prompt",async()=>{
    vi.mocked(labApi.chat).mockResolvedValue(labState());
    await ready();
    fireEvent.change(screen.getByRole("combobox",{name:"More modes"}),{target:{value:"library_actions"}});
    await waitFor(()=>expect(screen.getByRole("combobox",{name:"More modes"})).toHaveValue("library_actions"));
    type("Vary reach while returning to the tip.");fireEvent.click(screen.getByRole("button",{name:"Send"}));
    await waitFor(()=>expect(labApi.chat).toHaveBeenCalledOnce());
    expect(vi.mocked(labApi.chat).mock.calls[0][0]).toMatchObject({method:"library_actions",prompt:"actions-prompt",schema_guided:true});
    expect(labApi.start).not.toHaveBeenCalled();expect(labApi.reset).not.toHaveBeenCalled();
  });
  it("sends with Enter while preserving Shift+Enter and input composition",async()=>{
    vi.mocked(labApi.chat).mockResolvedValue(labState());
    await ready();
    const input=screen.getByRole("textbox",{name:"Message"});
    fireEvent.change(input,{target:{value:"Keep pace unchanged"}});
    fireEvent.keyDown(input,{key:"Enter",shiftKey:true});
    fireEvent.keyDown(input,{key:"Enter",isComposing:true,keyCode:229});
    expect(labApi.chat).not.toHaveBeenCalled();
    fireEvent.keyDown(input,{key:"Enter"});
    await waitFor(()=>expect(labApi.chat).toHaveBeenCalledOnce());
    expect(labApi.start).not.toHaveBeenCalled();
  });
  it("sends quick requests and earlier messages with one click",async()=>{
    const next=labState();next.revision=1;next.turns=[trial("Keep the base busy")];
    vi.mocked(labApi.chat).mockResolvedValue(next);
    await ready();
    fireEvent.click(screen.getByRole("button",{name:"Deeper"}));
    await waitFor(()=>expect(labApi.chat).toHaveBeenCalledWith(expect.objectContaining({message:"Deeper",method:"layered"}),expect.any(AbortSignal)));
    fireEvent.click(await screen.findByRole("button",{name:"Send again"}));
    await waitFor(()=>expect(labApi.chat).toHaveBeenCalledTimes(2));
    expect(vi.mocked(labApi.chat).mock.calls[1][0]).toMatchObject({message:"Keep the base busy",revision:1});
    expect(screen.getByRole("textbox",{name:"Message"})).toHaveValue("");
  });
  it("compares one request across the main modes without changing the chat or motion",async()=>{
    const result:LabCompareResult={trial:trial("Go deeper",{reply:"All the way down.",changed:["strokes"]}),perceptual:{position_min_percent:0,position_max_percent:70,pace:{effective_percent:20}},trace:[40,0,70,0]};
    vi.mocked(labApi.compare).mockResolvedValue(result);
    await ready();
    type("Go deeper");fireEvent.click(screen.getByRole("button",{name:"Compare modes"}));
    await waitFor(()=>expect(labApi.compare).toHaveBeenCalledTimes(5));
    expect(vi.mocked(labApi.compare).mock.calls.map(call=>call[0].method)).toEqual(["creative_v2","layered","stroke_ends","groove","plain_words"]);
    expect((await screen.findAllByText("Reach 0–70")).length).toBe(5);
    expect(labApi.chat).not.toHaveBeenCalled();expect(labApi.start).not.toHaveBeenCalled();expect(labApi.session).not.toHaveBeenCalled();
  });
  it("loads the relative edit prompt and matching schema while keeping the result in preview",async()=>{
    vi.mocked(labApi.chat).mockResolvedValue(labState());
    await ready();
    fireEvent.change(screen.getByRole("combobox",{name:"More modes"}),{target:{value:"edits"}});
    await waitFor(()=>expect(screen.getByRole("combobox",{name:"More modes"})).toHaveValue("edits"));
    type("Five points slower; preserve both layers.");fireEvent.click(screen.getByRole("button",{name:"Send"}));await waitFor(()=>expect(labApi.chat).toHaveBeenCalledOnce());
    expect(vi.mocked(labApi.chat).mock.calls[0][0]).toMatchObject({method:"edits",prompt:"edit-prompt",schema_guided:true});expect(labApi.start).not.toHaveBeenCalled();
  });
  it("cancels generation and keeps the draft without starting motion",async()=>{
    vi.mocked(labApi.chat).mockImplementation((_body,signal)=>new Promise((_resolve,reject)=>signal?.addEventListener("abort",()=>reject(new DOMException("Aborted","AbortError")))));
    await ready();
    type("Change reach gradually");
    fireEvent.click(screen.getByRole("button",{name:"Send"}));
    fireEvent.click(await screen.findByRole("button",{name:"Cancel generation"}));
    await screen.findByText("Generation canceled. The draft was kept.");
    expect(screen.getByRole("textbox",{name:"Message"})).toHaveValue("Change reach gradually");
    expect(labApi.start).not.toHaveBeenCalled();
  });
  it("loads an observation as an editable draft without sending it",async()=>{
    const used=vi.fn();render(<LLMLab initialDraft="Observation: the plotted range changed abruptly" draftUsed={used}/>);
    await screen.findByDisplayValue("Observation: the plotted range changed abruptly");
    expect(used).toHaveBeenCalled();expect(labApi.chat).not.toHaveBeenCalled();expect(labApi.start).not.toHaveBeenCalled();
  });
  it("starts Autopilot with the same mode and prompt without enabling live motion",async()=>{
    vi.mocked(labApi.session).mockResolvedValue({...labState(),session:{active:true,live:false,autopilot:true,method:"layered",prompt:"layered-prompt",model:"local-model",schema_guided:true,interval_seconds:20}});
    await ready();
    fireEvent.click(screen.getByRole("switch",{name:"Autopilot"}));
    await screen.findByText("Preview test running");
    expect(labApi.session).toHaveBeenCalledWith(expect.objectContaining({live:false,autopilot:true,method:"layered",prompt:"layered-prompt"}));
    expect(labApi.start).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button",{name:"Configure"}));
    expect(screen.getAllByRole("link",{name:"Help"}).some(link=>link.getAttribute("href")==="#/labs/help/autopilot")).toBe(true);
  });
});

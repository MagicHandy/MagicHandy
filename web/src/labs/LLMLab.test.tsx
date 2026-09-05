import {fireEvent,render,screen,waitFor} from "@testing-library/react";
import {beforeEach,describe,expect,it,vi} from "vitest";
import {labApi} from "./api";
import {labLimits,labPreview,labState} from "./fixtures";
import {LLMLab} from "./LLMLab";

const app=vi.hoisted(()=>({state:{settings:{motion:{}}},backendOnline:true,readOnly:false,refresh:vi.fn(),show:vi.fn()}));
vi.mock("../state/app-state",()=>({useAppState:()=>app,useToast:()=>({show:app.show})}));
vi.mock("../api/client",()=>({api:{stopMotion:vi.fn()}}));
vi.mock("./api",async importOriginal=>({...await importOriginal<typeof import("./api")>(),labApi:{state:vi.fn(),status:vi.fn(),preview:vi.fn(),chat:vi.fn(),reset:vi.fn(),start:vi.fn(),session:vi.fn()}}));
describe("separate LLM Lab",()=>{
  beforeEach(()=>{vi.clearAllMocks();app.state.settings.motion=labLimits;app.readOnly=false;
	vi.mocked(labApi.status).mockResolvedValue({revision:0,busy:false});
    vi.mocked(labApi.state).mockResolvedValue(labState());vi.mocked(labApi.preview).mockImplementation(async spec=>labPreview(spec));vi.mocked(labApi.start).mockResolvedValue({});});
  it("keeps chat as preview until the user explicitly starts a live session",async()=>{
    const next=labState();next.revision=1;next.current.anchor_percent=100;
    next.turns=[{message:"Hold the tip",reply:"Tip anchor in preview.",raw:'{"controls":{"anchor_percent":100}}',valid:true,changed:["anchor_percent"],model:"local-model",method:"controls",prompt:"control-prompt",elapsed_ms:500,provider_calls:1,before:labState().current,after:next.current}];
    vi.mocked(labApi.chat).mockResolvedValue(next);
    render(<LLMLab/>);await screen.findByText("Describe the motion you want to test");
    fireEvent.change(screen.getByRole("textbox",{name:"Message"}),{target:{value:"Hold the tip"}});fireEvent.click(screen.getByRole("button",{name:"Send"}));
    await screen.findByText("Tip anchor in preview.");expect(labApi.start).not.toHaveBeenCalled();
    vi.mocked(labApi.session).mockResolvedValue({...next,session:{active:true,live:true,autopilot:false,method:"edits",prompt:"edit-prompt",model:"local-model",schema_guided:true,interval_seconds:20}});
    fireEvent.click(screen.getByRole("checkbox",{name:"Live motion"}));
    fireEvent.click(screen.getByRole("button",{name:"Start test"}));
    await screen.findByText("Live test running");
    expect(labApi.session).toHaveBeenCalledWith(expect.objectContaining({live:true,autopilot:false,method:"layered"}));
    expect(screen.getByRole("combobox",{name:"Test mode"})).toBeDisabled();
    expect(vi.mocked(labApi.chat).mock.calls[0][0]).toMatchObject({schema_guided:true,revision:0,method:"layered"});
  });
  it("keeps generation disabled for read-only clients",async()=>{
    app.readOnly=true;render(<LLMLab/>);await screen.findByText("Describe the motion you want to test");
    expect(screen.getByRole("button",{name:"Send"})).toBeDisabled();expect(screen.getByRole("button",{name:"Start test"})).toBeDisabled();
  });
  it("sends the selected library naming contract with its matching prompt",async()=>{
    vi.mocked(labApi.chat).mockResolvedValue(labState());
    render(<LLMLab/>);await screen.findByText("Describe the motion you want to test");
    fireEvent.change(screen.getByRole("combobox",{name:"Test mode"}),{target:{value:"library"}});
    fireEvent.change(screen.getByRole("combobox",{name:"Test mode"}),{target:{value:"library_actions"}});
    fireEvent.change(screen.getByRole("textbox",{name:"Message"}),{target:{value:"Vary reach while returning to the tip."}});
    fireEvent.click(screen.getByRole("button",{name:"Send"}));
    await waitFor(()=>expect(labApi.chat).toHaveBeenCalledOnce());
    expect(vi.mocked(labApi.chat).mock.calls[0][0]).toMatchObject({method:"library_actions",prompt:"actions-prompt",schema_guided:true});
    expect(labApi.start).not.toHaveBeenCalled();
  });
  it("sends with Enter while preserving Shift+Enter and input composition",async()=>{
    vi.mocked(labApi.chat).mockResolvedValue(labState());
    render(<LLMLab/>);await screen.findByText("Describe the motion you want to test");
    const input=screen.getByRole("textbox",{name:"Message"});
    fireEvent.change(input,{target:{value:"Keep pace unchanged"}});
    fireEvent.keyDown(input,{key:"Enter",shiftKey:true});
    fireEvent.keyDown(input,{key:"Enter",isComposing:true,keyCode:229});
    expect(labApi.chat).not.toHaveBeenCalled();
    fireEvent.keyDown(input,{key:"Enter"});
    await waitFor(()=>expect(labApi.chat).toHaveBeenCalledOnce());
    expect(labApi.start).not.toHaveBeenCalled();
  });
  it("loads the relative edit prompt and matching schema while keeping the result in preview",async()=>{
    const state=labState();state.prompts.edits="edit-prompt";vi.mocked(labApi.state).mockResolvedValue(state);vi.mocked(labApi.chat).mockResolvedValue(state);
    render(<LLMLab/>);await screen.findByText("Describe the motion you want to test");
    fireEvent.change(screen.getByRole("combobox",{name:"Test mode"}),{target:{value:"edits"}});
    fireEvent.change(screen.getByRole("textbox",{name:"Message"}),{target:{value:"Five points slower; preserve both layers."}});
    fireEvent.click(screen.getByRole("button",{name:"Send"}));await waitFor(()=>expect(labApi.chat).toHaveBeenCalledOnce());
    expect(vi.mocked(labApi.chat).mock.calls[0][0]).toMatchObject({method:"edits",prompt:"edit-prompt",schema_guided:true});expect(labApi.start).not.toHaveBeenCalled();
  });
  it("cancels generation and keeps the draft without starting motion",async()=>{
    vi.mocked(labApi.chat).mockImplementation((_body,signal)=>new Promise((_resolve,reject)=>signal?.addEventListener("abort",()=>reject(new DOMException("Aborted","AbortError")))));
    render(<LLMLab/>);await screen.findByText("Describe the motion you want to test");
    fireEvent.change(screen.getByRole("textbox",{name:"Message"}),{target:{value:"Change reach gradually"}});
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
    render(<LLMLab/>);await screen.findByText("Describe the motion you want to test");
    fireEvent.click(screen.getByRole("checkbox",{name:"Autopilot"}));fireEvent.click(screen.getByRole("button",{name:"Start test"}));
    await screen.findByText("Preview test running");
    expect(labApi.session).toHaveBeenCalledWith(expect.objectContaining({live:false,autopilot:true,method:"layered",prompt:"layered-prompt"}));
    expect(labApi.start).not.toHaveBeenCalled();
    expect(screen.getAllByRole("link",{name:"Help"}).some(link=>link.getAttribute("href")==="#/labs/help/autopilot")).toBe(true);
  });
});

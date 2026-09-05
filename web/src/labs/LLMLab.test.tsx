import {fireEvent,render,screen,waitFor} from "@testing-library/react";
import {beforeEach,describe,expect,it,vi} from "vitest";
import {labApi} from "./api";
import {labLimits,labPreview,labState} from "./fixtures";
import {LLMLab} from "./LLMLab";

const app=vi.hoisted(()=>({state:{settings:{motion:{}}},backendOnline:true,readOnly:false,refresh:vi.fn(),show:vi.fn()}));
vi.mock("../state/app-state",()=>({useAppState:()=>app,useToast:()=>({show:app.show})}));
vi.mock("../api/client",()=>({api:{stopMotion:vi.fn()}}));
vi.mock("./api",async importOriginal=>({...await importOriginal<typeof import("./api")>(),labApi:{state:vi.fn(),preview:vi.fn(),chat:vi.fn(),reset:vi.fn(),start:vi.fn()}}));
describe("separate LLM Lab",()=>{
  beforeEach(()=>{vi.clearAllMocks();app.state.settings.motion=labLimits;app.readOnly=false;
    vi.mocked(labApi.state).mockResolvedValue(labState());vi.mocked(labApi.preview).mockImplementation(async spec=>labPreview(spec));vi.mocked(labApi.start).mockResolvedValue({});});
  it("keeps accepted model proposals as previews until an explicit audition",async()=>{
    const next=labState();next.revision=1;next.current.anchor_percent=100;
    next.turns=[{message:"Hold the tip",reply:"Tip anchor in preview.",raw:'{"controls":{"anchor_percent":100}}',valid:true,changed:["anchor_percent"],model:"local-model",method:"controls",prompt:"control-prompt",elapsed_ms:500,provider_calls:1,before:labState().current,after:next.current}];
    vi.mocked(labApi.chat).mockResolvedValue(next);
    render(<LLMLab/>);await screen.findByDisplayValue("local-model");
    fireEvent.change(screen.getByRole("textbox",{name:"Message"}),{target:{value:"Hold the tip"}});fireEvent.click(screen.getByRole("button",{name:"Send"}));
    await screen.findByText("Tip anchor in preview.");expect(labApi.start).not.toHaveBeenCalled();
    await waitFor(()=>expect(screen.getByRole("button",{name:"Audition proposal"})).toBeEnabled());
    fireEvent.click(screen.getByRole("button",{name:"Audition proposal"}));expect(labApi.start).toHaveBeenCalledOnce();
    await waitFor(()=>expect(screen.getByRole("button",{name:"Audition proposal"})).toBeEnabled());
    expect(vi.mocked(labApi.chat).mock.calls[0][0]).toMatchObject({schema_guided:false,revision:0,method:"controls"});
  });
  it("keeps generation disabled for read-only clients",async()=>{
    app.readOnly=true;render(<LLMLab/>);await screen.findByDisplayValue("local-model");
    expect(screen.getByRole("button",{name:"Send"})).toBeDisabled();expect(screen.getByRole("button",{name:"Audition proposal"})).toBeDisabled();
  });
  it("sends the selected library naming contract with its matching prompt",async()=>{
    vi.mocked(labApi.chat).mockResolvedValue(labState());
    render(<LLMLab/>);await screen.findByDisplayValue("local-model");
    fireEvent.click(screen.getByText("Experiment setup"));
    fireEvent.change(screen.getByRole("combobox",{name:"Control interface"}),{target:{value:"library"}});
    fireEvent.change(screen.getByRole("combobox",{name:"Recipe naming"}),{target:{value:"library_actions"}});
    fireEvent.change(screen.getByRole("textbox",{name:"Message"}),{target:{value:"Vary reach while returning to the tip."}});
    fireEvent.click(screen.getByRole("button",{name:"Send"}));
    await waitFor(()=>expect(labApi.chat).toHaveBeenCalledOnce());
    expect(vi.mocked(labApi.chat).mock.calls[0][0]).toMatchObject({method:"library_actions",prompt:"actions-prompt",schema_guided:true});
    expect(labApi.start).not.toHaveBeenCalled();
  });
  it("sends with Enter while preserving Shift+Enter and input composition",async()=>{
    vi.mocked(labApi.chat).mockResolvedValue(labState());
    render(<LLMLab/>);await screen.findByDisplayValue("local-model");
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
    render(<LLMLab/>);await screen.findByDisplayValue("local-model");fireEvent.click(screen.getByText("Experiment setup"));
    fireEvent.change(screen.getByRole("combobox",{name:"Control interface"}),{target:{value:"edits"}});
    fireEvent.change(screen.getByRole("textbox",{name:"Message"}),{target:{value:"Five points slower; preserve both layers."}});
    fireEvent.click(screen.getByRole("button",{name:"Send"}));await waitFor(()=>expect(labApi.chat).toHaveBeenCalledOnce());
    expect(vi.mocked(labApi.chat).mock.calls[0][0]).toMatchObject({method:"edits",prompt:"edit-prompt",schema_guided:true});expect(labApi.start).not.toHaveBeenCalled();
  });
  it("cancels generation and keeps the draft without starting motion",async()=>{
    vi.mocked(labApi.chat).mockImplementation((_body,signal)=>new Promise((_resolve,reject)=>signal?.addEventListener("abort",()=>reject(new DOMException("Aborted","AbortError")))));
    render(<LLMLab/>);await screen.findByDisplayValue("local-model");
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
});

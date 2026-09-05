import {fireEvent,render,screen} from "@testing-library/react";
import {beforeEach,describe,expect,it,vi} from "vitest";
import {initialFlow,labApi,type LabObservation,type LabObservations} from "./api";
import {labLimits} from "./fixtures";
import {ObservationEditor,ObservationsPage} from "./Observations";

const app=vi.hoisted(()=>({backendOnline:true,readOnly:false}));
vi.mock("../state/app-state",()=>({useAppState:()=>app}));
vi.mock("./api",async importOriginal=>({...await importOriginal<typeof import("./api")>(),labApi:{observations:vi.fn(),saveObservation:vi.fn(),deleteObservation:vi.fn(),chat:vi.fn(),start:vi.fn()}}));
const row:LabObservation={id:"observation-1",created_at:"2026-09-04T12:00:00Z",source:"motion",method:"flow",label:"Motion preview",spec:initialFlow,settings:labLimits,text:"The plotted range change is too abrupt."};
const records:LabObservations={observations:[row],storage_path:"C:/review/magichandy.db",capacity:200};

describe("saved lab observations",()=>{
  beforeEach(()=>{vi.clearAllMocks();app.readOnly=false;app.backendOnline=true;vi.mocked(labApi.observations).mockResolvedValue(records);vi.mocked(labApi.saveObservation).mockResolvedValue(records);});
  it("saves the captured preview only on request and identifies its local storage",async()=>{
    const target={source:"motion" as const,spec:initialFlow,settings_key:"saved-limits",method:"flow"};
    render(<ObservationEditor target={target} label="Continuous flow · 5–95 · 25%" close={vi.fn()}/>);
    fireEvent.change(screen.getByRole("textbox",{name:"Your observation"}),{target:{value:row.text}});
    expect(labApi.saveObservation).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button",{name:"Save observation"}));
    await screen.findByText(records.storage_path);
    expect(labApi.saveObservation).toHaveBeenCalledWith(target,row.text);
    expect(screen.getByRole("link",{name:"View saved observations"})).toHaveAttribute("href","#/labs/observations");
    expect(labApi.chat).not.toHaveBeenCalled();expect(labApi.start).not.toHaveBeenCalled();
  });
  it("keeps a failed save as an unsaved draft",async()=>{
    vi.mocked(labApi.saveObservation).mockRejectedValue(new Error("the lab conversation changed; select the reply again"));
    render(<ObservationEditor target={{source:"llm",settings_key:"limits",revision:1,turn_index:0}} label="An earlier reply" close={vi.fn()}/>);
    fireEvent.change(screen.getByRole("textbox",{name:"Your observation"}),{target:{value:row.text}});
    fireEvent.click(screen.getByRole("button",{name:"Save observation"}));
    await screen.findByRole("alert");expect(screen.getByRole("textbox",{name:"Your observation"})).toHaveValue(row.text);
    expect(screen.queryByText("Observation saved.")).not.toBeInTheDocument();
  });
  it("reads durable records and passes feedback to a draft only after Use in chat",async()=>{
    const useInChat=vi.fn();render(<ObservationsPage useInChat={useInChat}/>);
    await screen.findByText(row.text);expect(useInChat).not.toHaveBeenCalled();
    expect(screen.getByText(records.storage_path)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button",{name:"Use in chat"}));
    expect(useInChat).toHaveBeenCalledWith(expect.stringContaining(row.text));
    expect(labApi.chat).not.toHaveBeenCalled();expect(labApi.start).not.toHaveBeenCalled();
  });
  it("requires an explicit delete and renders the returned backend collection",async()=>{
    vi.mocked(labApi.deleteObservation).mockResolvedValue({...records,observations:[]});
    render(<ObservationsPage useInChat={vi.fn()}/>);await screen.findByText(row.text);
    fireEvent.click(screen.getByRole("button",{name:"Delete"}));expect(labApi.deleteObservation).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button",{name:"Confirm delete"}));
    await screen.findByText("No observations yet");expect(labApi.deleteObservation).toHaveBeenCalledWith(row.id);
  });
  it("keeps observation mutations disabled for readers",async()=>{
    app.readOnly=true;render(<ObservationsPage useInChat={vi.fn()}/>);await screen.findByText(row.text);
    expect(screen.getByRole("button",{name:"Delete"})).toBeDisabled();
    expect(screen.getByRole("button",{name:"Use in chat"})).toBeDisabled();
    expect(labApi.deleteObservation).not.toHaveBeenCalled();
  });
});

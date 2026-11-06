import { afterEach, expect, test, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import { ExecutionContext } from "../app/execution";
import { DestinationSetup } from "./DestinationSetup";

afterEach(() => vi.restoreAllMocks());
function renderSetup(path: string, capabilities = ["destinations", "credentials"]) {
 const client = new QueryClient({defaultOptions:{queries:{retry:false}}});
 render(<QueryClientProvider client={client}><ExecutionContext.Provider value={{mode:"cloud",user:{id:"owner",name:"Owner"},workspace:"workspace",capabilities}}><MemoryRouter initialEntries={[path]}><DestinationSetup /></MemoryRouter></ExecutionContext.Provider></QueryClientProvider>);
}
const response = (data: unknown) => new Response(JSON.stringify({apiVersion:"ding.ing/v1alpha1",data}),{status:200});
test("a mismatched setup link cannot read or change destination setup", async () => {
 const fetch = vi.spyOn(globalThis,"fetch");
 renderSetup("/destination-setup?account=someone-else&workspace=workspace");
 expect(await screen.findByText("This setup link belongs to another account or workspace")).toBeVisible();
 expect(fetch).not.toHaveBeenCalled();
 expect(screen.queryByText("Create notification destination")).toBeNull();
});
test("cloud setup saves credentials only to the protected API and creates only a destination", async () => {
 const writes: {path:string;body:Record<string,unknown>}[]=[];
 let saved=false;
 vi.spyOn(globalThis,"fetch").mockImplementation(async(input,options)=>{
  const path=String(input),body=options?.body?JSON.parse(String(options.body)):{};
  if(options?.method==="POST")writes.push({path,body});
  if(path==="/v1/cloud/secrets") {if(options?.method==="POST")saved=true;return response({names:saved?["DING_ALERTS"]:[]});}
  if(path.startsWith("/v1/console/destinations?"))return response({destinations:[],more:false,cursor:""});
  if(path.startsWith("/v1/onboarding/destination-status?"))return response({accountID:"owner",workspaceID:"workspace",destinationRef:new URL("https://ding.example"+path).searchParams.get("destination"),state:"missing",setupURL:"/ui/destination-setup"});
  if(path==="/v1/onboarding/destination")return response({});
  throw new Error("Unexpected request "+path);
 });
 renderSetup("/destination-setup?account=owner&workspace=workspace");
 fireEvent.change(await screen.findByLabelText("Destination ID"),{target:{value:"slack-alerts"}});
 fireEvent.change(screen.getByLabelText("Delivery type"),{target:{value:"slack"}});
 fireEvent.change(await screen.findByLabelText("Destination webhook URL"),{target:{value:"https://hooks.example/private-value"}});
 fireEvent.click(screen.getByRole("button",{name:"Save destination securely"}));
 await screen.findByText("This workspace credential is saved. Its value stays out of chat.");
 fireEvent.click(screen.getByRole("button",{name:"Create notification destination"}));
 await screen.findByText("This saved destination is missing. Select another destination or create one.");
 expect(writes).toEqual([
  {path:"/v1/cloud/secrets",body:{name:"DING_ALERTS",value:"https://hooks.example/private-value"}},
  {path:"/v1/onboarding/destination",body:{accountID:"owner",workspaceID:"workspace",id:"slack-alerts",type:"slack",credential:"DING_ALERTS"}},
 ]);
 expect(screen.queryByDisplayValue("https://hooks.example/private-value")).toBeNull();
});
test("inspect-only users cannot confirm an accepted notification", async () => {
 vi.spyOn(globalThis,"fetch").mockImplementation(async input => response(String(input).startsWith("/v1/onboarding") ? {accountID:"owner",workspaceID:"workspace",destinationRef:"alerts",revision:"revision",state:"accepted",probe:{id:"probe",outcome:"accepted"}} : {destinations:[],more:false}));
 renderSetup("/destination-setup?destination=alerts",["inspect"]);
 expect(await screen.findByRole("button",{name:"I received the test at this destination"})).toBeDisabled();
});

test("a lost replacement-test response reuses its new key while the old receipt is stale", async () => {
 sessionStorage.clear();
 const keys:string[]=[];
 vi.spyOn(globalThis,"fetch").mockImplementation(async(input,options)=>{
  const path=String(input);
  if(path==="/v1/cloud/destination-test") { keys.push(JSON.parse(String(options?.body)).operationKey); throw new TypeError("response lost"); }
  if(path.startsWith("/v1/onboarding"))return response({accountID:"owner",workspaceID:"workspace",destinationRef:"alerts",revision:"new-revision",state:"stale",probe:{id:"previous-test",outcome:"accepted"}});
  return response({destinations:[],more:false});
 });
 renderSetup("/destination-setup?destination=alerts");
 fireEvent.click(await screen.findByRole("button",{name:"Send a new labeled test"}));
 const retry=await screen.findByRole("button",{name:"Check this test outcome"});
 await vi.waitFor(()=>expect(retry).toBeEnabled());
 fireEvent.click(retry);
 await vi.waitFor(()=>expect(keys).toHaveLength(2));
 expect(keys[0]).not.toBe("previous-test");expect(keys[1]).toBe(keys[0]);
});

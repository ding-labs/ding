import { expect, test } from "vitest";
import { render, screen } from "@testing-library/react";
import { SignIn } from "./SignIn";

test("OIDC sign-in carries only setup coordinates back to the Console", () => {
 const before=location.href;
 try {
  history.replaceState({},"","/ui/destination-setup?account=account&workspace=workspace&destination=alerts&return=https://evil.example&sign_in=canceled");
  render(<SignIn config={{access:"shared",execution:"cloud",methods:[{id:"oidc",label:"Sign in with GitHub",url:"/auth/login"}]}} connected={()=>{}} />);
  const login=new URL(screen.getByRole("link",{name:"Sign in with GitHub"}).getAttribute("href")!,location.origin);
  expect(login.pathname).toBe("/auth/login");
  const back=new URL(login.searchParams.get("return")!,location.origin);
  expect(back.pathname).toBe("/ui/destination-setup");
  expect([...back.searchParams]).toEqual([["account","account"],["workspace","workspace"],["destination","alerts"]]);
 } finally {history.replaceState({},"",before);}
});

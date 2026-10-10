import { createRoot } from "react-dom/client";
import { Workspace } from "./Workspace";
import { hostBridge } from "./bridge";
import "../../console/src/theme.css";
import "./styles.css";

createRoot(document.getElementById("root")!).render(
  <Workspace bridge={hostBridge()} />,
);

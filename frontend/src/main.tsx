import { createRoot } from "react-dom/client";
import { App } from "./App";
import { initializeAndroid } from "./android";
import "./style.css";
import "./android.css";
initializeAndroid();
createRoot(document.getElementById("root")!).render(<App />);

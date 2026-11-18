import parser from "@babel/eslint-parser";
import hooks from "eslint-plugin-react-hooks";

export default [
  {
    files: ["src/**/*.{ts,tsx}"],
    languageOptions: {
      parser,
      parserOptions: {
        requireConfigFile: false,
        babelOptions: { parserOpts: { plugins: ["typescript", "jsx"] } },
      },
    },
    plugins: { "react-hooks": hooks },
    rules: { "react-hooks/rules-of-hooks": "error" },
  },
  {
    files: ["src/devices/{runtime.tsx,useDeviceObservation.ts}", "src/state/app-state.tsx", "src/shell/StopButton.tsx"],
    rules: { "react-hooks/exhaustive-deps": "error" },
  },
];

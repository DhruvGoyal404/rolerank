import tseslint from "typescript-eslint";

export default tseslint.config(
  { ignores: ["dist/**", "*.mjs", "playwright-report/**", "test-results/**"] },
  ...tseslint.configs.strictTypeChecked,
  {
    languageOptions: {
      parserOptions: { projectService: true, tsconfigRootDir: import.meta.dirname },
    },
    rules: {
      "no-restricted-syntax": [
        "error",
        {
          selector: "JSXAttribute[name.name='dangerouslySetInnerHTML']",
          message: "Third-party and model text renders as text, never as HTML.",
        },
      ],
    },
  },
);

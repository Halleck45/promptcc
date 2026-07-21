// Storybook stories are skipped entirely: docs prose and display fixtures.
export default {
  parameters: {
    docs: {
      description: {
        component:
          "If the assistant streams a long answer, this component renders it incrementally. When the stream ends, it settles. Never blocks the main thread while animating the text.",
      },
    },
  },
};

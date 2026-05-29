export default function (plop) {
  plop.setGenerator('feature', {
    description: 'Scaffold a new feature slice under src/features/',
    prompts: [
      {
        type: 'input',
        name: 'name',
        message: 'Feature name (kebab-case, e.g. "notifications"):',
        validate: (v) => /^[a-z][a-z0-9-]*$/.test(v) || 'Use lowercase kebab-case',
      },
    ],
    actions: [
      {
        type: 'add',
        path: 'src/features/{{name}}/api/{{name}}.api.ts',
        templateFile: 'plop-templates/feature/api.ts.hbs',
      },
      {
        type: 'add',
        path: 'src/features/{{name}}/queryKeys.ts',
        templateFile: 'plop-templates/feature/queryKeys.ts.hbs',
      },
      {
        type: 'add',
        path: 'src/features/{{name}}/index.ts',
        templateFile: 'plop-templates/feature/index.ts.hbs',
      },
      { type: 'add', path: 'src/features/{{name}}/components/.gitkeep', template: '' },
      { type: 'add', path: 'src/features/{{name}}/hooks/.gitkeep', template: '' },
      { type: 'add', path: 'src/features/{{name}}/types/.gitkeep', template: '' },
    ],
  })
}

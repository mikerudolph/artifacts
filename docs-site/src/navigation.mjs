export const sections = [
  {
    id: 'core', label: 'Core', href: '',
    groups: [
      { label: 'Start here', items: [
        { label: 'Introduction', slug: '' },
        { label: 'Quickstart', slug: 'getting-started' },
        { label: 'Core concepts', slug: 'core/concepts' },
      ] },
      { label: 'Build with Artifacts', items: [
        { label: 'Model your data', slug: 'core/data-model' },
        { label: 'Integrate your application', slug: 'core/integration' },
        { label: 'Authentication', slug: 'core/authentication' },
        { label: 'Manage repositories', slug: 'core/repositories' },
        { label: 'Write files', slug: 'core/writing-files' },
        { label: 'Read files & history', slug: 'core/reading-files' },
        { label: 'Work with Git', slug: 'core/git' },
        { label: 'Forks & imports', slug: 'core/forks-and-imports' },
      ] },
      { label: 'Reference & operations', items: [
        { label: 'REST API reference', slug: 'core/api-reference' },
        { label: 'Errors & limits', slug: 'core/errors-and-limits' },
        { label: 'Configuration', slug: 'core/configuration' },
        { label: 'Production deployment', slug: 'core/deployment' },
        { label: 'Storage architecture', slug: 'storage' },
      ] },
    ],
  },
  {
    id: 'examples', label: 'Examples', href: 'examples',
    groups: [{ label: 'Patterns in practice', items: [
      { label: 'Choose an example', slug: 'examples' },
      { label: 'Agent sessions', slug: 'examples/agent-sessions' },
      { label: 'Personal memory with MCP', slug: 'examples/personal-memory' },
      { label: 'Snapshot handoffs', slug: 'examples/snapshot-handoffs' },
    ] }],
  },
  {
    id: 'developer-tools', label: 'Developer tools', href: 'developer-tools',
    groups: [{ label: 'Inspect & develop', items: [
      { label: 'Overview', slug: 'developer-tools' },
      { label: 'Web UI', slug: 'developer-tools/web-ui' },
      { label: 'Local development', slug: 'developer-tools/local-development' },
    ] }],
  },
];

export function sectionFor(id) {
  return sections.find((section) => section.id !== 'core' &&
    (id === section.id || id.startsWith(`${section.id}/`))) || sections[0];
}

export function docsHref(slug = '', base = '/artifacts') {
  return `${base.replace(/\/$/, '')}/${slug ? `${slug.replace(/\/$/, '')}/` : ''}`;
}

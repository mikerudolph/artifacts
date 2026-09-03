import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

export default defineConfig({
  site: 'https://mikerudolph.github.io',
  base: '/artifacts',
  integrations: [
    starlight({
      title: 'Artifacts',
      description: 'Git-compatible artifact repositories built for agents and sessions.',
      disable404Route: true,
      customCss: ['./src/styles/custom.css'],
      head: [
        {
          tag: 'meta',
          attrs: {
            property: 'og:image',
            content: 'https://mikerudolph.github.io/artifacts/og.png',
          },
        },
        {
          tag: 'meta',
          attrs: { property: 'og:image:alt', content: 'Artifacts documentation' },
        },
        {
          tag: 'meta',
          attrs: { name: 'twitter:card', content: 'summary_large_image' },
        },
      ],
      editLink: {
        baseUrl: 'https://github.com/mikerudolph/artifacts/edit/main/docs/',
      },
      markdown: {
        processedDirs: ['../docs'],
      },
      social: [
        {
          icon: 'github',
          label: 'GitHub',
          href: 'https://github.com/mikerudolph/artifacts',
        },
      ],
      sidebar: [
        {
          label: 'Start here',
          items: [
            { label: 'Overview', slug: '' },
            { label: 'Get started', slug: 'getting-started' },
          ],
        },
        {
          label: 'Guides',
          items: [{ label: 'Agent onboarding', slug: 'onboarding' }],
        },
        {
          label: 'Architecture',
          items: [{ label: 'Storage', slug: 'storage' }],
        },
      ],
    }),
  ],
});

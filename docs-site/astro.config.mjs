import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import { sections } from './src/navigation.mjs';

export default defineConfig({
  site: 'https://mikerudolph.github.io',
  base: '/artifacts',
  redirects: { '/onboarding': '/artifacts/examples/agent-sessions/' },
  integrations: [
    starlight({
      title: 'Artifacts',
      description: 'Durable, Git-compatible file repositories for your applications and agents.',
      disable404Route: true,
      favicon: '/favicon.svg',
      customCss: ['./src/styles/custom.css', './src/styles/orbital.css'],
      components: {
        Header: './src/components/Header.astro',
        Sidebar: './src/components/Sidebar.astro',
        PageTitle: './src/components/PageTitle.astro',
        Hero: './src/components/Hero.astro',
        Footer: './src/components/Footer.astro',
        ThemeProvider: './src/components/ThemeProvider.astro',
      },
      expressiveCode: {
        themes: ['tokyo-night', 'github-light'],
        styleOverrides: { borderRadius: '2px', codeFontSize: '0.8125rem', codeLineHeight: '1.8' },
      },
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
      sidebar: sections.map(({ label, groups }) => ({ label, items: groups })),
    }),
  ],
});

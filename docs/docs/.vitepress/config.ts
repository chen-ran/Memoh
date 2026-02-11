import { defineConfig } from 'vitepress'

// https://vitepress.vuejs.org/config/app-configs
export default defineConfig({
  title: 'Memoh Documentation',
  description: 'Multi-Member, Structured Long-Memory, Containerized AI Agent System.',

  head: [
    ['link', { rel: 'icon', href: '/logo.png' }]
  ],

  base: '/',

  locales: {
    root: {
      label: 'English',
      lang: 'en'
    },
    zh: {
      label: '简体中文',
      lang: 'zh',
    }
  },

  themeConfig: {
    siteTitle: 'Memoh',
    sidebar: {
      '/': [
        {
          text: 'Overview',
          link: '/index.md'
        },
        {
          text: 'Getting Started',
          link: '/getting-started.md'
        },
        {
          text: 'Core Concepts',
          items: [
            {
              text: 'Concepts Overview',
              link: '/concepts/index.md'
            },
            {
              text: 'Accounts and Linking',
              link: '/concepts/identity-and-binding.md'
            }
          ]
        },
        {
          text: 'Documentation Style Guide',
          items: [
            {
              text: 'Terminology Rules',
              link: '/style/terminology.md'
            }
          ]
        }
      ],
      '/zh/': [
        {
          text: '文档总览',
          link: '/zh/index.md'
        },
        {
          text: '核心概念',
          items: [
            {
              text: '概念总览',
              link: '/zh/concepts/index.md'
            },
            {
              text: '账号模型与绑定',
              link: '/zh/concepts/identity-and-binding.md'
            }
          ]
        },
        {
          text: '文档写作规范',
          items: [
            {
              text: '术语规范',
              link: '/zh/style/terminology.md'
            }
          ]
        }
      ]
    },

    logo: {
      src: '/logo.png',
      alt: 'Memoh'
    },
    
    socialLinks: [
      { icon: 'github', link: 'https://github.com/memohai/Memoh' }
    ],
    
    footer: {
      message: 'Published under AGPLv3',
      copyright: 'Copyright © 2024 Memoh'
    },
    
    search: {
      provider: 'local'
    },
    
    editLink: {
      pattern: 'https://github.com/memohai/Memoh/edit/main/docs/docs/:path',
      text: 'Edit on GitHub'
    },
    
    lastUpdated: {
      text: 'Last Updated',
      formatOptions: {
        dateStyle: 'short',
        timeStyle: 'medium'
      }
    }
  },

  ignoreDeadLinks: true,
})

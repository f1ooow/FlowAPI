import { Link } from '@tanstack/react-router'
import { ArrowRight } from 'lucide-react'
/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useCallback, useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { RichContent } from '@/components/rich-content'
import { Button } from '@/components/ui/button'
import { useTheme } from '@/context/theme-provider'
import { useSystemConfig } from '@/hooks/use-system-config'
import { isLikelyHtml } from '@/lib/content-format'
import { useAuthStore } from '@/stores/auth-store'

import { useHomePageContent } from './hooks'

export function Home() {
  const { i18n, t } = useTranslation()
  const iframeRef = useRef<HTMLIFrameElement>(null)
  const { resolvedTheme } = useTheme()
  const { systemName } = useSystemConfig()
  const { auth } = useAuthStore()
  const isAuthenticated = !!auth.user
  const { content, isLoaded, isUrl } = useHomePageContent()

  const syncIframePreferences = useCallback(() => {
    try {
      iframeRef.current?.contentWindow?.postMessage(
        { themeMode: resolvedTheme },
        '*'
      )
      iframeRef.current?.contentWindow?.postMessage(
        { lang: i18n.language },
        '*'
      )
    } catch {
      // Cross-origin frames may reject access while navigating.
    }
  }, [i18n.language, resolvedTheme])

  useEffect(() => {
    if (isUrl) {
      syncIframePreferences()
    }
  }, [isUrl, syncIframePreferences])

  if (!isLoaded) {
    return (
      <PublicLayout showMainContainer={false} showHeader={false}>
        <main className='flex min-h-screen items-center justify-center'>
          <div className='text-muted-foreground'>{t('Loading...')}</div>
        </main>
      </PublicLayout>
    )
  }

  if (content) {
    if (isUrl) {
      return (
        <PublicLayout showMainContainer={false}>
          {/*
            allow-top-navigation-by-user-activation: the custom home page URL is
            admin-configured (trusted); this lets its target="_top" nav/menu links
            navigate the top-level window on user click. The default sandbox blocks
            this on desktop, while some mobile browsers allow it via allow-popups,
            causing inconsistent behavior. This token only permits user-activated
            top-level navigation and does NOT grant same-origin access.
          */}
          <iframe
            ref={iframeRef}
            src={content}
            className='h-screen w-full border-none'
            title={t('Custom Home Page')}
            sandbox='allow-forms allow-popups allow-popups-to-escape-sandbox allow-scripts allow-top-navigation-by-user-activation'
            onLoad={syncIframePreferences}
          />
        </PublicLayout>
      )
    }

    const contentIsHtml = isLikelyHtml(content)

    if (contentIsHtml) {
      return (
        <PublicLayout showMainContainer={false}>
          <RichContent
            mode='html'
            htmlVariant='isolated'
            content={content}
            className='custom-home-content'
          />
        </PublicLayout>
      )
    }

    return (
      <PublicLayout>
        <div className='mx-auto max-w-6xl px-4 py-8'>
          <RichContent
            mode='markdown'
            content={content}
            className='custom-home-content'
          />
        </div>
      </PublicLayout>
    )
  }

  return (
    <PublicLayout
        showMainContainer={false}
        showHeader={false}
      siteName={systemName}
      headerProps={{
        showNavigation: false,
        showNotifications: false,
        showAuthButtons: false,
      }}
    >
      <main className='relative flex min-h-svh items-center overflow-hidden bg-[#f7f9fc] px-6 pt-16'>
        <section className='mx-auto grid w-full max-w-[1280px] items-center gap-14 py-12 lg:grid-cols-12 lg:gap-10 lg:py-16'>
          <div className='landing-animate-fade-up mx-auto flex w-full max-w-2xl flex-col items-start opacity-0 lg:col-span-6 lg:mx-0'>
            <div className='inline-flex h-8 items-center gap-2 rounded-full border border-blue-200 bg-blue-50 px-3 text-xs font-medium text-blue-700'>
              <span className='size-1.5 rounded-full bg-blue-500' />
              {t('Internal AI API infrastructure')}
            </div>

            <h1 className='text-foreground mt-7 text-[44px] leading-[1.12] font-semibold tracking-normal sm:text-[52px] lg:text-[58px]'>
              {t('A unified API gateway, built for')}
              <span className='mt-2 block text-blue-600'>
                {t("Your team's AI traffic")}
              </span>
            </h1>

            <p className='text-muted-foreground mt-7 max-w-xl text-base leading-8 sm:text-lg'>
              {t(
                'Connect services through one standard interface, with requests, routes, and usage managed in one place.'
              )}
            </p>

            <Button
              size='lg'
              className='mt-10 h-12 gap-2 rounded-md bg-blue-600 px-7 text-white hover:bg-blue-700'
              render={<Link to={isAuthenticated ? '/dashboard' : '/sign-in'} />}
            >
              {isAuthenticated ? t('Go to Dashboard') : t('Sign in')}
              <ArrowRight className='size-4' aria-hidden='true' />
            </Button>
          </div>

          <div
            className='landing-animate-fade-up hidden opacity-0 lg:col-span-6 lg:block'
            style={{ animationDelay: '100ms' }}
          >
            <div className='border-border bg-background overflow-hidden rounded-lg border shadow-[0_24px_70px_-36px_rgba(37,99,235,0.28)]'>
              <div className='border-foreground/10 flex h-13 items-center border-b px-5'>
                <div className='flex h-full items-center gap-7 text-sm'>
                  <span className='flex h-full items-center border-b-2 border-blue-600 font-semibold text-blue-700'>
                    {t('Request')}
                  </span>
                  <span className='text-muted-foreground'>{t('Routing')}</span>
                  <span className='text-muted-foreground'>{t('Usage')}</span>
                </div>
                <div className='text-muted-foreground ml-auto flex items-center gap-2 font-mono text-[11px]'>
                  <span className='size-1.5 rounded-full bg-emerald-500' />
                  200 OK
                </div>
              </div>

              <div className='border-foreground/10 flex h-15 items-center gap-3 border-b px-6 font-mono text-xs'>
                <span className='rounded bg-emerald-50 px-2 py-1 font-semibold text-emerald-700'>
                  POST
                </span>
                <span className='text-foreground/80'>/v1/chat/completions</span>
              </div>

              <div className='border-foreground/10 min-h-64 border-b px-7 py-6 font-mono text-[12px] leading-6 sm:text-[13px]'>
                <p className='text-muted-foreground mb-4 text-[10px] font-semibold tracking-normal uppercase'>
                  {t('Request')}
                </p>
                <p>
                  <span className='font-semibold text-emerald-600'>curl</span>{' '}
                  <span className='text-blue-600'>-X</span> POST{' '}
                  <span className='text-amber-700'>
                    &quot;/v1/chat/completions&quot;
                  </span>{' '}
                  {'\\'}
                </p>
                <p className='pl-4'>
                  <span className='text-blue-600'>-H</span>{' '}
                  <span className='text-amber-700'>
                    &quot;Authorization: Bearer sk-••••&quot;
                  </span>{' '}
                  {'\\'}
                </p>
                <p className='pl-4'>
                  <span className='text-blue-600'>-d</span>{' '}
                  <span className='text-foreground/70'>&apos;&#123;</span>
                </p>
                <p className='text-foreground/70 pl-8'>
                  <span className='text-blue-700'>&quot;model&quot;</span>:{' '}
                  <span className='text-amber-700'>&quot;your-model&quot;</span>
                  ,
                </p>
                <p className='text-foreground/70 pl-8'>
                  <span className='text-blue-700'>&quot;messages&quot;</span>:
                  [&#123; &quot;role&quot;: &quot;user&quot;,
                </p>
                <p className='text-foreground/70 pl-12'>
                  &quot;content&quot;:{' '}
                  <span className='text-amber-700'>&quot;...&quot;</span>{' '}
                  &#125;]
                </p>
                <p className='text-foreground/70 pl-4'>&#125;&apos;</p>
              </div>

              <div className='min-h-44 px-7 py-6 font-mono text-[12px] leading-6 sm:text-[13px]'>
                <p className='text-muted-foreground mb-4 text-[10px] font-semibold tracking-normal uppercase'>
                  {t('Response')}
                </p>
                <p className='text-foreground/70'>&#123;</p>
                <p className='text-foreground/70 pl-4'>
                  &quot;choices&quot;: [&#123; &quot;message&quot;: &#123;
                </p>
                <p className='text-foreground/70 pl-8'>
                  &quot;content&quot;:{' '}
                  <span className='text-emerald-700'>
                    &quot;Request routed.&quot;
                  </span>
                </p>
                <p className='text-foreground/70 pl-4'>&#125; &#125;],</p>
                <p className='text-foreground/70 pl-4'>
                  &quot;usage&quot;: &#123; &quot;total_tokens&quot;: 27 &#125;
                </p>
                <p className='text-foreground/70'>&#125;</p>
              </div>

              <div className='border-foreground/10 text-muted-foreground flex h-11 items-center gap-3 border-t px-6 font-mono text-[10px] uppercase'>
                <span>142 ms</span>
                <span className='bg-foreground/20 size-1 rounded-full' />
                <span>27 tokens</span>
                <span className='bg-foreground/20 size-1 rounded-full' />
                <span>{t('Request completed')}</span>
              </div>
            </div>
          </div>
        </section>
      </main>
    </PublicLayout>
  )
}

import { createRouter, createWebHistory } from 'vue-router'
import MissionControlView from '../views/MissionControlView.vue'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/',
      name: 'mission-control',
      component: MissionControlView,
    },
    {
      path: '/projects',
      name: 'projects-setup',
      component: () => import('../views/ProjectSetupView.vue'),
    },
    {
      path: '/connectors',
      name: 'connectors',
      component: () => import('../views/ConnectorsView.vue'),
    },
    {
      path: '/tasks/:id',
      name: 'task-detail',
      component: () => import('../views/TaskDetailView.vue'),
      props: true,
    },
    {
      path: '/analytics',
      name: 'analytics',
      component: () => import('../views/AnalyticsView.vue'),
    },
    {
      path: '/tools',
      name: 'tool-hub',
      component: () => import('../views/ToolHubView.vue'),
    },
    {
      path: '/skills',
      name: 'skills-hub',
      component: () => import('../views/SkillsView.vue'),
    },
    {
      path: '/settings/providers',
      name: 'provider-settings',
      component: () => import('../views/ProviderSettingsView.vue'),
    },
    {
      path: '/settings/workflows',
      name: 'workflow-settings',
      component: () => import('../views/WorkflowSettingsView.vue'),
    },
    {
      path: '/settings/registries',
      redirect: { path: '/skills', query: { tab: 'prompts' } },
    },
    {
      path: '/:pathMatch(.*)*',
      redirect: '/',
    },
  ],
})

export default router

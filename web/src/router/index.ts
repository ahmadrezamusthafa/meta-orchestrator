import { createRouter, createWebHistory } from 'vue-router'
import MissionControlView from '../views/MissionControlView.vue'
import TaskDetailView from '../views/TaskDetailView.vue'
import ToolHubView from '../views/ToolHubView.vue'
import ProviderSettingsView from '../views/ProviderSettingsView.vue'
import WorkflowSettingsView from '../views/WorkflowSettingsView.vue'
import RegistriesSettingsView from '../views/RegistriesSettingsView.vue'
import ProjectSetupView from '../views/ProjectSetupView.vue'
import ConnectorsView from '../views/ConnectorsView.vue'

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
      component: ProjectSetupView,
    },
    {
      path: '/connectors',
      name: 'connectors',
      component: ConnectorsView,
    },
    {
      path: '/tasks/:id',
      name: 'task-detail',
      component: TaskDetailView,
      props: true,
    },
    {
      path: '/tools',
      name: 'tool-hub',
      component: ToolHubView,
    },
    {
      path: '/settings/providers',
      name: 'provider-settings',
      component: ProviderSettingsView,
    },
    {
      path: '/settings/workflows',
      name: 'workflow-settings',
      component: WorkflowSettingsView,
    },
    {
      path: '/settings/registries',
      name: 'registries-settings',
      component: RegistriesSettingsView,
    },
  ],
})

export default router

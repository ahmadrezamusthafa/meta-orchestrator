import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useLayoutStore = defineStore('layout', () => {
  const isNavExpanded = ref(false)

  function toggleNav() {
    isNavExpanded.value = !isNavExpanded.value
  }

  function setNavExpanded(expanded: boolean) {
    isNavExpanded.value = expanded
  }

  return {
    isNavExpanded,
    toggleNav,
    setNavExpanded,
  }
})

import { createRouter, createWebHistory } from 'vue-router'
import TargetList from '../views/TargetList.vue'
import TargetDetail from '../views/TargetDetail.vue'

const routes = [
  { path: '/', name: 'list', component: TargetList },
  { path: '/targets/:id', name: 'detail', component: TargetDetail, props: true }
]

export default createRouter({
  history: createWebHistory(),
  routes
})

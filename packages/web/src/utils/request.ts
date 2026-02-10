import axios, { type AxiosRequestConfig } from 'axios'
import router from '@/router'

const axiosInstance = axios.create({
  baseURL: '/api',
})

axiosInstance.interceptors.request.use((config) => {
  const token = localStorage.getItem('token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

axiosInstance.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error?.response?.status === 401) {
      router.replace({ name: 'Login' })
    }
    return Promise.reject(error)
  },
)

export default function request(config: AxiosRequestConfig) {
  return axiosInstance(config)
}

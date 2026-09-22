// Filename: context/AuthContext.tsx
"use client";

import React, { createContext, useContext, useState, useEffect, ReactNode } from 'react';
import { useRouter } from 'next/navigation';
import { AxiosError } from 'axios';
import * as api from '@/lib/api';
import { User, LoginCredentials } from '@/lib/api';

interface AuthContextType {
  user: User | null;
  isAuthenticated: boolean;
  login: (credentials: LoginCredentials) => Promise<void>;
  logout: () => Promise<void>; // Make logout return a Promise
  loading: boolean;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export const AuthProvider = ({ children }: { children: ReactNode }) => {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
  const router = useRouter();

  useEffect(() => {
    const checkAuthStatus = async () => {
      try {
        const response = await api.getMe();
        setUser(response.data);
      } catch (error) {
        console.log(error)
        setUser(null);
      } finally {
        setLoading(false);
      }
    };
    checkAuthStatus();
  }, []);

  const login = async (credentials: LoginCredentials) => {
    try {
      await api.login(credentials);
      const response = await api.getMe();
      setUser(response.data);
      router.push('/dashboard');
    } catch (error) {
      console.error("Login failed", error);
      const axiosError = error as AxiosError<{ error: string }>;
      const message = axiosError.response?.data?.error || "An unknown error occurred.";
      throw new Error(message);
    }
  };

  // +++ UPDATE THIS FUNCTION +++
  const logout = async () => {
    try {
      await api.logout(); // Call the backend to clear the cookie
    } catch (error) {
      console.error("Logout failed", error);
    } finally {
      // Always clear frontend state and redirect, even if API call fails
      setUser(null);
      router.push('/login');
    }
  };

  const isAuthenticated = !!user;

  return (
    <AuthContext.Provider value={{ user, isAuthenticated, login, logout, loading }}>
      {!loading && children}
    </AuthContext.Provider>
  );
};

export const useAuth = () => {
  const context = useContext(AuthContext);
  if (context === undefined) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
};
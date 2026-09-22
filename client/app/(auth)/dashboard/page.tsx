// Filename: app/(auth)/dashboard/page.tsx
"use client";

import { useState, useEffect } from 'react';
import * as api from '@/lib/api';
import { Project } from '@/lib/api';
import { ProjectCard } from '@/components/dashboard/ProjectCard';
import { ProjectCardSkeleton } from '@/components/dashboard/ProjectCardSkeleton';

export default function DashboardPage() {
  const [projects, setProjects] = useState<Project[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const fetchProjects = async () => {
      try {
        const response = await api.getProjects();
        setProjects(response.data);
      } catch (err) {
        setError("Failed to fetch projects. Please try again later.");
        console.error(err);
      } finally {
        setIsLoading(false);
      }
    };

    fetchProjects();
  }, []);

  const renderContent = () => {
    if (isLoading) {
      return (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {Array.from({ length: 3 }).map((_, index) => (
            <ProjectCardSkeleton key={index} />
          ))}
        </div>
      );
    }

    if (error) {
      return <p className="text-red-500">{error}</p>;
    }

    if (projects.length === 0) {
      return (
        <div className="text-center py-10 border-2 border-dashed rounded-lg">
          <h3 className="text-lg font-semibold">No Projects Found</h3>
          <p className="text-muted-foreground mt-2">
            Get started by creating a new project.
          </p>
        </div>
      );
    }

    return (
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {projects.map((project) => (
          <ProjectCard key={project.id} project={project} />
        ))}
      </div>
    );
  };

  return (
    <div className="max-w-7xl mx-auto py-6 sm:px-6 lg:px-8">
      <div className="px-4 py-6 sm:px-0">
        <h2 className="text-2xl font-bold mb-4">Your Projects</h2>
        {renderContent()}
      </div>
    </div>
  );
}
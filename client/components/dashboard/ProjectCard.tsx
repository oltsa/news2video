// Filename: components/dashboard/ProjectCard.tsx
"use client";

import Link from 'next/link';
import { Card, CardHeader, CardTitle } from '@/components/ui/card';
import { FolderKanban } from 'lucide-react';
import { Project } from '@/lib/api';

interface ProjectCardProps {
  project: Project;
}

export function ProjectCard({ project }: ProjectCardProps) {
  return (
    <Link
      href={`/projects/${project.id}`}
      className="block hover:scale-[1.02] transition-transform duration-200"
    >
      <Card>
        <CardHeader className="flex flex-row items-center gap-4">
          <FolderKanban className="h-8 w-8 text-muted-foreground" />
          <CardTitle>{project.project_key}</CardTitle>
        </CardHeader>
      </Card>
    </Link>
  );
}
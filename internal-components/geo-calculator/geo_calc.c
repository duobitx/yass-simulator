#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <math.h>
#include <time.h>
#include <signal.h>
#include <unistd.h>
#include <sys/mman.h>
#include <sys/stat.h>    
#include <fcntl.h>  

#include "TLE.h"
#include "common.h"

#define SHM_NAME "/geo_calc_shared_memory"

double radiusearthkm = 6378.137;
int n_sat=0, cont_flag=1;
time_t tm0,tm;
TLE sat_orb[MAXSAT];
struct common *pcom;

int read_orbs()
{
    char line[256];
    char *str = NULL;
    FILE *in_file = NULL;
    TLE *psat;
    double r[3],v[3];

    n_sat=0; 
    
    if( (in_file = fopen("tle_100.txt","r")) == NULL ) { fprintf(stderr, "no such file\n"); return 1; }
    
    while(fgets(line,255,in_file) != NULL)
    {
        if(line[0]=='1')
        {
            if( n_sat >= MAXSAT ) { fprintf(stderr,"MAXSAT exceeded\n"); return 1; }
            psat=sat_orb+n_sat;
            strncpy(psat->line1,line,69); psat->line1[69]=0;
            fgets(line,255,in_file);
            strncpy(psat->line2,line,69); psat->line2[69]=0;
            parseLines(psat,psat->line1,psat->line2);
            getRVForDate(psat, 1000* tm0, r, v); 
            if( !psat->sgp4Error )  n_sat++;; 
        }
    }

    fclose(in_file);

    return 0;
}

double dist(int s1, int s2)
{ struct sat_pos_dscr *ps1, *ps2;
  double t, dx, dy, dz, d2, x, y, z;
  
  ps1=pcom->sat + s1; ps2=pcom->sat + s2;
  dx=ps2->x - ps1->x; dy=ps2->y - ps1->y; dz=ps2->z - ps1->z; d2 = dx*dx + dy*dy + dz*dz;
  
  t=( -ps1->x*dx + -ps1->y*dy + -ps1->z*dz ) / d2;
    
  //printf("%2d %2d %f %f\n",s1,s2,t, sqrt(d2));
  if( t<= 0.0 || t >= 1.0 ) return sqrt(d2);
  x=ps1->x+t*dx; y=ps1->y+t*dy; z=ps1->z+t*dz;
  if( sqrt(x*x + y*y + z*z ) <= radiusearthkm ) return -1;
  return sqrt(d2);
    
}

int refcmp(const void *p1, const void *p2)
{
  if( ((struct sat_ref_dscr *)p1)->dist < ((struct sat_ref_dscr *)p2)->dist ) return -1;
  if( ((struct sat_ref_dscr *)p1)->dist > ((struct sat_ref_dscr *)p2)->dist ) return 1;
  return 0;
}


int calc_pos()
{ double r[4], v[3], d;
  struct sat_pos_dscr *psat;
  struct tm *info;
  int k,l; //, sn,ss;
  
  pcom->busy=1; pcom->nsat=n_sat;
  info = gmtime( &tm );
  sprintf(pcom->utc_dttm,"%4d-%02d-%02d %02d:%02d:%02d\n",info->tm_year+1900,info->tm_mon+1,info->tm_mday,info->tm_hour,info->tm_min,info->tm_sec);

  for(k=0;k<n_sat;k++) {
     getRVForDate(sat_orb+k, 1000* tm, r, v); if( sat_orb[k].sgp4Error ) fprintf(stderr,"Alert %d,  sgp4Error=%d\n",k+1,sat_orb[k].sgp4Error);
     r[3]=sqrt(r[0]*r[0]+r[1]*r[1]+r[2]*r[2]) - radiusearthkm;
     memcpy(&pcom->sat[k].x,r,4*sizeof(double));
   }
  //ss=0;
  for(k=0;k<n_sat;k++) { psat=pcom->sat + k;
    psat->nref=0;
    for(l=0;l<n_sat;l++) if( k != l && (d=dist(k,l)) > 0.0 ) { psat->sat_ref[psat->nref].sid=l; psat->sat_ref[psat->nref++].dist=d;}
    //qsort(psat->sat_ref,psat->nref,sizeof(struct sat_ref_dscr),refcmp);
    //ss+=pinf->nref;
   }
   //printf("srednio %d\n",ss/n_sat);
  pcom->busy=0; 
  return 0;
}

static void int_hnd()
{
cont_flag=0;
}


int main()
{
  struct tm *info;
  TLE *psat;
  struct sat_inf_dscr *pinf;
  time_t t0,t1,t2;
  double r[3],v[3];
  int k,l, shm;

  time(&t0);  tm0=t1=t0; info = gmtime( &t0 );
  printf("%4d-%02d-%02d %02d:%02d:%02d\n",info->tm_year+1900,info->tm_mon+1,info->tm_mday,info->tm_hour,info->tm_min,info->tm_sec);
  
  if( read_orbs() ) exit(4);
  printf("nsat=%d\n",n_sat);
  
  shm = shm_open(SHM_NAME, O_CREAT | O_RDWR, 0666);
  if( shm == -1 ) {  perror("shm_open"); return 1; }
  ftruncate(shm, sizeof(struct common));

  pcom= mmap(0, sizeof(struct common), PROT_WRITE, MAP_SHARED, shm, 0);
  if(pcom == MAP_FAILED) {   perror("mmap");   return 1;  }
  memset(pcom,0,sizeof(struct common)); 	

  signal(7,int_hnd);

  while(1) {
      time(&t2);
      if(t1 != t2 ) {
        tm = t1=t2;
        calc_pos();
        //info = gmtime( &tm );
        //printf("%4d-%02d-%02d %02d:%02d:%02d\n",info->tm_year+1900,info->tm_mon+1,info->tm_mday,info->tm_hour,info->tm_min,info->tm_sec);
      }
      usleep(200000); // 200 ms
      if( !cont_flag ) { printf("signal INT received\n"); break; }
    }
  //for(k=0;k<n_sat;k++)
   // printf("%3d %10.2f %10.2f %10.2f %10.2f %3d\n", k+1, pcom->sat[k].x, pcom->sat[k].y,pcom->sat[k].z,pcom->sat[k].h,pcom->sat[k].nref);  
  shm_unlink(SHM_NAME);
  return 0;
}

// gcc -o geo_calc SGP4.c TLE.c geo_calc.c -lm -lrt

#include <time.h>
#include <cstring>
#include <vector>
#include <iostream>
#include <fstream>
#include <sys/mman.h>
#include <sys/stat.h>    
#include <sys/types.h>
#include <fcntl.h>  
#include <unistd.h>
#include <signal.h>

#include "CoordGeodetic.h"
#include "SGP4.h"
#include "common.h"

#define SHM_NAME "/geo_calc_shared_memory"

struct common comm, *pcom;

double radiusearthkm = 6378.137;


int n_sat=0, n_bs=1, cont_flag=1;
time_t tm, tm_start;
float accel=1.0;


std::vector<libsgp4::Tle>  V_tle;
std::vector<libsgp4::SGP4> V_sgp4;
libsgp4::DateTime sgp4dttm;
	
	
int read_tle(const char* infile)
{
  std::ifstream file;
  bool got_first_line = false;
  std::string line, line1, line2;

  file.open(infile);
  if (!file.is_open()) {  std::cerr << "Error opening file" << std::endl; return 1; }

  while (!file.eof()) {
    std::getline(file, line);
    libsgp4::Util::Trim(line);
    if (line.length() == 0 || line[0] == '#') { got_first_line = false;  continue;  }
    if (!got_first_line) {
      try  {
         if (line.length() >= libsgp4::Tle::LineLength()) {
         got_first_line = true;
         line1 = line;
        }
       }
      catch (libsgp4::TleException& e) {
         std::cerr << "Error: " << e.what() << std::endl;
         std::cerr << line << std::endl;
       }
     }
    else {
      got_first_line = false;
      line2 = line.substr(0, libsgp4::Tle::LineLength());
      try {
        if (line.length() >= libsgp4::Tle::LineLength()) {
        	libsgp4::Tle tle = libsgp4::Tle(line1, line2);
        	libsgp4::SGP4 sgp4(tle);
        	libsgp4::Eci eci = sgp4.FindPosition(sgp4dttm);
        	n_sat++; 
        	V_tle.push_back(tle);
        	V_sgp4.push_back(sgp4);
         }        
       }
      catch (libsgp4::TleException& e) {
        std::cerr << "Error: " << e.what() << std::endl;
        std::cerr << line << std::endl;
       }
      catch (libsgp4::SatelliteException& e) {
        std::cerr << "Error: " << e.what() << std::endl;
        std::cerr << line << std::endl;
       }
      catch (libsgp4::DecayedException& e)
        {
            std::cerr << e.what() << std::endl;
        }
    }
  }

  file.close();
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

int calc_pos()
{ libsgp4::Vector pos;
	struct sat_pos_dscr *psat;
  struct tm *info;
  double  d, PI = 3.1415926;
  int k,l; 
  
  pcom->busy=1; pcom->nsat=n_sat; pcom->nbs=n_bs;
  info = gmtime( &tm );
  sprintf(pcom->utc_dttm,"%4d-%02d-%02d %02d:%02d:%02d\n",info->tm_year+1900,info->tm_mon+1,info->tm_mday,info->tm_hour,info->tm_min,info->tm_sec);
  sgp4dttm = libsgp4::DateTime(info->tm_year+1900,info->tm_mon+1,info->tm_mday,info->tm_hour,info->tm_min,info->tm_sec);
  
  for(k=0;k<n_sat;k++) { psat=pcom->sat + k;
    libsgp4::Eci eci =  V_sgp4[k].FindPosition(sgp4dttm); pos=eci.Position();
    psat->x=pos.x; psat->y=pos.y; psat->z=pos.z;
    //psat->h=sqrt(pos.x*pos.x+pos.y*pos.y+pos.z*pos.z) - radiusearthkm;
    libsgp4::CoordGeodetic geo = eci.ToGeodetic();
    psat->lat=180.0*geo.latitude/PI; psat->lon=180.0*geo.longitude/PI; psat->alt=geo.altitude;
   }

  psat=pcom->sat + n_sat;  psat->lat=52.164313494928955; psat->lon=21.022268460902644; psat->alt=0.118; 
  for(k=n_sat;k<n_sat+n_bs;k++) { psat=pcom->sat + k;
  	libsgp4::Eci bs = libsgp4::Eci(sgp4dttm,psat->lat, psat->lon, psat->alt);
    pos = bs.Position();  psat->x=pos.x; psat->y=pos.y; psat->z=pos.z;
   } 		
  
  for(k=0;k<n_sat+n_bs;k++) { psat=pcom->sat + k;
    psat->nref=0;
    for(l=0;l<n_sat;l++) if( k != l && (d=dist(k,l)) > 0.0 ) { psat->sat_ref[psat->nref].sid=l; psat->sat_ref[psat->nref++].dist=d;}
    //qsort(psat->sat_ref,psat->nref,sizeof(struct sat_ref_dscr),refcmp);
   }
  pcom->busy=0; 
  //std::cout << sgp4dttm << " " << pcom->sat[n_sat].nref <<  " " << pcom->sat[n_sat].x <<  " " << pcom->sat[n_sat].y << std::endl;	
  return 0;
}

static void int_hnd(int s)
{
cont_flag=0;
}

static int dttmverif(char *arg)
{ struct tm tm;
  time_t t;;
  int d,m,y,h,mn;

  if( strlen(arg) != 16 || arg[4] != '-' || arg[7] != '-' || arg[10] != 'T' || arg[13] != ':' ) return 1;
  h=mn=0;
  sscanf(arg,"%4d-%2d-%2dT%2d:%2d",&y,&m,&d,&h,&mn);
  tm.tm_mday = d; tm.tm_mon = m-1; tm.tm_year=  y-1900; tm.tm_hour=h; tm.tm_min=mn; tm.tm_sec=tm.tm_isdst=0; 
  t=timegm(&tm);
  if( tm.tm_mday != d || tm.tm_mon+1 != m || tm.tm_year+1900 != y || tm.tm_hour != h || tm.tm_min != mn ) return 1;
  tm_start=t;
  return 0;
}



int main(int argc, char **argv)
{   libsgp4::Vector pos;
    const char* file_name = "tle_100.txt";
    time_t t0,t1,t2;
    int k, shm;
    char *pc;
    
    time(&tm_start);
    if( argc > 1 ) {
    	if( dttmverif(argv[1]) ) { fprintf(stderr,"wrong literal %s\n",argv[1]); exit(8); }
    	if( argc > 2 ) { accel=strtof(argv[2],&pc); if( *pc || accel < 1.0 )  {  fprintf(stderr,"Wrong acceleration factor\n"); exit(8); } }
     }

    sgp4dttm = libsgp4::DateTime(2025,10,1);

    if( read_tle(file_name) ) exit(8);
    std::cout << "n_sat= " << n_sat << std::endl;
    	
    shm = shm_open(SHM_NAME, O_CREAT | O_RDWR, 0666);
    if( shm == -1 ) {  perror("shm_open"); return 1; }
    ftruncate(shm, sizeof(struct common));

    pcom=(struct common*) mmap(0, sizeof(struct common), PROT_WRITE, MAP_SHARED, shm, 0);
    if(pcom == MAP_FAILED) {   perror("mmap");   return 1;  }
    memset(pcom,0,sizeof(struct common)); 	

    signal(7,int_hnd);
    signal(2,int_hnd);
   	
    time(&t0); t1=t0;
    while(1) {
      if( !cont_flag ) { printf("signal received\n"); break; }
      usleep(200000); // 200 ms
      time(&t2);
      if(t1 != t2 ) {
        t1 = t2;
        tm=tm_start + (time_t)(accel*(t2-t0));
        calc_pos();
      }
    }
	
    shm_unlink(SHM_NAME);
    return 0;
}

// g++ -o geo_calc geo_calc.cc   -L. -lsgp4
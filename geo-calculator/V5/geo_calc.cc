#include <time.h>
#include <cstring>
#include <vector>
#include <jansson.h>
#include <iostream>
#include <fstream>
#include <iomanip>
#include <ctime>
#include <string.h>
#include <unistd.h>
#include <signal.h>
#include <fcntl.h>
#include <sys/mman.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <vector>

//#include <sstream>

#include "CoordGeodetic.h"
#include "SGP4.h"
#include "common.h"

#define SHM_NAME "/geo_calc_shared_memory"
struct common comm, *pcom;

int n_sat=0, r_sat=0,  n_bs=0, cont_flag=1;
time_t tm_sym, tm_start, tm_stop;
float accel=1.0;


std::vector<libsgp4::Tle>  V_tle;
std::vector<libsgp4::SGP4> V_sgp4;
libsgp4::DateTime sgp4dttm;

double radiusearthkm = 6378.137;

char (*tmp)[MAXSAT][3][70];
struct bs_dscr { char name[32]; double lat,lon,alt; };
std::vector<struct bs_dscr>  V_bs;

int parse_sat(json_t *sat)
{ json_error_t error;
  size_t index;
  json_t *line;
  std::string str;

  json_t *sname = json_object_get(sat, "Name");
   if( !json_is_string(sname) ) { std::cerr << "  satelite name not defined\n"; return 1; }
   //std::cout << "  satelita " << json_string_value(sname) << "\n";
   str=json_string_value(sname); strcpy((*tmp)[r_sat][0],str.substr(0,31).c_str());
  json_t *tle = json_object_get(sat, "TLE");
   if( !json_is_array(tle) ) { std::cerr << "  TLE for " << json_string_value(sname) << " is not an array\n"; return 1; }
   if( json_array_size(tle) != 2 ) { std::cerr << "  wrong  TLE for " << json_string_value(sname) << "\n"; return 1; }
  json_array_foreach(tle, index, line) {
    if( !json_is_string(line) ) { std::cerr << "  wrong  TLE for " << json_string_value(sname) << "\n"; return 1;  }
    str=json_string_value(line); if( str.length() > 69 ) { std::cerr << "  too long  TLE lines for " << json_string_value(sname) << "\n"; return 1;  }
    strcpy((*tmp)[r_sat][index+1],str.c_str());
   }
  r_sat++;
  return 0;
}

int parse_bs(json_t *sat)
{ json_error_t error;
  struct bs_dscr bs;
  std::string str;

  json_t *bsname = json_object_get(sat, "Name");
   if( !json_is_string(bsname) ) { std::cerr << "  station name not defined\n"; return 1; }
   //std::cout << "  stacja " << json_string_value(bsname) << "\n";
   str=json_string_value(bsname); strcpy(bs.name,str.substr(0,31).c_str());
  json_t *geo = json_object_get(sat, "Geo");
   if( !json_is_object(geo) ) { std::cerr << "  Geo for " << json_string_value(bsname) << " not defined\n"; return 1; }
  json_t *lat = json_object_get(geo, "Lat");
   if( json_is_integer(lat) ) bs.lat = json_integer_value(lat);
    else if( json_is_real(lat) ) bs.lat = json_real_value(lat);
    else { std::cerr << "  Lat for " << json_string_value(bsname) << " not defined\n"; return 1; }
  json_t *lon = json_object_get(geo, "Lng");
   if( json_is_integer(lon) ) bs.lon = json_integer_value(lon);
    else if( json_is_real(lon) ) bs.lon = json_real_value(lon);
    else { std::cerr << "  Long for " << json_string_value(bsname) << " not defined\n"; return 1; }
  json_t *alt = json_object_get(geo, "Alt");
   if( alt && !json_is_null(alt) ) {
     if( json_is_integer(alt) ) bs.alt = json_integer_value(alt);
      else if( json_is_real(alt) ) bs.alt = json_real_value(alt);
    }
   else bs.alt=0.0;

  V_bs.push_back(bs); n_bs++;
  return 0;
}

int parse_json(char *fname)
{ json_error_t error;
  std::tm tm = {};
  size_t index;
  json_t *sat;
  int n;

  json_t *root = json_load_file(fname, 0, &error);
   if (!root) {  std::cerr << "  " << error.text << "\n"; return 1; }
  json_t *ename = json_object_get(root, "Name");
   if( json_is_string(ename) ) std::cout << "  experiment name:  " << json_string_value(ename) << "\n";
  json_t *estart = json_object_get(root, "StartTime");
   if( !json_is_string(estart) ) { std::cerr << "  StartTime not defined\n"; return 1; }
   std::istringstream ss(json_string_value(estart));
   ss >> std::get_time(&tm, "%Y-%m-%dT%H:%M:%S");
   if (ss.fail()) { std::cerr << "  wrong StartTime literal: " << json_string_value(estart) << "\n"; return 1; }
    //tm.tm_mday != d || tm.tm_mon+1 != m || tm.tm_year+1900 != y
     sgp4dttm = libsgp4::DateTime(tm.tm_year+1900,tm.tm_mon+1,tm.tm_mday);

   tm_start=timegm(&tm);
  json_t *edur = json_object_get(root, "MaxDuration");
   if( json_is_integer(edur) ) tm_stop = tm_start + json_integer_value(edur);
  json_t *acc = json_object_get(root, "TimeAcceleration");
   if( json_is_integer(acc) ) accel= json_integer_value(acc);
    else if( json_is_real(acc) ) accel= json_real_value(acc);

  json_t *flota = json_object_get(root, "FsNodes");
   if( !json_is_array(flota) ) {  std::cerr << "  'FsNodes' is not a JSON table.\n"; return 1; }
   if( (n=json_array_size(flota)) > MAXSAT ) {  std::cerr << "  " << n << " exceeds MAXSAT.\n"; return 1; }
  json_array_foreach(flota, index, sat) {
    json_t *tle = json_object_get(sat, "TLE");
    if( tle && !json_is_null(tle) ) { if( parse_sat(sat) ) return 1; }
   }
  json_array_foreach(flota, index, sat) {
    json_t *tle = json_object_get(sat, "TLE");
    if( !tle || json_is_null(tle) ) { if( parse_bs(sat) ) return 1; }
   }

  json_decref(root);
  return 0;
}


int init_sgp4()
{ int k, tlelinelen=libsgp4::Tle::LineLength();


  for(k=0;k<r_sat;k++)
    if( strlen((*tmp)[k][1]) == tlelinelen && strlen((*tmp)[k][2])== tlelinelen ) {
      try {
          libsgp4::Tle tle = libsgp4::Tle((*tmp)[k][0],(*tmp)[k][1],(*tmp)[k][2]);
          libsgp4::SGP4 sgp4(tle);
          libsgp4::Eci eci = sgp4.FindPosition(sgp4dttm);
          n_sat++;
          V_tle.push_back(tle);
          V_sgp4.push_back(sgp4);
       }
      catch (libsgp4::TleException& e) {
        std::cerr << "  " << (*tmp)[k][0] << ": TLE Error: " << e.what() << std::endl;
       }
      catch (libsgp4::SatelliteException& e) {
        std::cerr << "  " << (*tmp)[k][0] << ": SGP$ Error: " << e.what() << std::endl;
       }
      catch (libsgp4::DecayedException& e) {
        std::cerr << "  " << (*tmp)[k][0] << ": " << e.what() << std::endl;
       }
     }
    else std::cerr << " satelite " << (*tmp)[k][0] << " ignored - wrong TLE lines length\n";

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
  info = gmtime( &tm_sym );
  sprintf(pcom->utc_dttm,"%4d-%02d-%02d %02d:%02d:%02d\n",info->tm_year+1900,info->tm_mon+1,info->tm_mday,info->tm_hour,info->tm_min,info->tm_sec);
  sgp4dttm = libsgp4::DateTime(info->tm_year+1900,info->tm_mon+1,info->tm_mday,info->tm_hour,info->tm_min,info->tm_sec);

  for(k=0;k<n_sat;k++) { psat=pcom->sat + k;
    strcpy(psat->name, V_tle[k].Name().c_str());
    libsgp4::Eci eci =  V_sgp4[k].FindPosition(sgp4dttm); pos=eci.Position();
    psat->x=pos.x; psat->y=pos.y; psat->z=pos.z;
    //psat->h=sqrt(pos.x*pos.x+pos.y*pos.y+pos.z*pos.z) - radiusearthkm;
    libsgp4::CoordGeodetic geo = eci.ToGeodetic();
    psat->lat=180.0*geo.latitude/PI; psat->lon=180.0*geo.longitude/PI; psat->alt=geo.altitude;
   }

  for(k=0;k<n_bs;k++) { psat=pcom->sat + n_sat + k;
    strcpy(psat->name, V_bs[k].name);
    psat->lat=V_bs[k].lat; psat->lon=V_bs[k].lon; psat->alt=V_bs[k].alt;
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


int main(int argc, char **argv)
{
  libsgp4::Vector pos;
  time_t t0,t1,t2;
  int k, shm;


  if( argc < 2 ) { std::cerr << "geo_calc <file name>\n"; exit(8); }
  tmp=(char(*)[MAXSAT][3][70])malloc(3*MAXSAT*70); if( !tmp ) { std::cout << "can't allocate memory\n"; exit(8); }

  std::cout << "Parsing file: " << argv[1] << "\n";
   if( parse_json(argv[1]) ) exit(8);
   std::cout << "  n_sat: " << std::setw(5) << r_sat << "\n  n_bs:  " << std::setw(5) << n_bs << "\n";

  std::cout << "Filling SGP4 structures\n";
   //sgp4dttm = libsgp4::DateTime(2027,10,1);
   if( init_sgp4() ) exit(8);
   std::cout << "  n_sat= " << n_sat << " (number of verified satelites)\n";
 
  free(tmp);
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
      tm_sym = tm_start + (time_t)(accel*(t2-t0));
      if( tm_stop > tm_start && tm_sym > tm_stop ) { printf("end of experimentn"); break; }
      calc_pos();
     }
   }
   
  /* 
  printf("%s\n",pcom->utc_dttm); 
  for(k=0;k<n_sat+n_bs;k++) 
   printf("%3d %-32s %10.2f %10.2f %10.2f     %10.2f %10.2f %10.2f    %d\n",k,
    pcom->sat[k].name,pcom->sat[k].x,pcom->sat[k].y,pcom->sat[k].z,pcom->sat[k].lat,pcom->sat[k].lon,pcom->sat[k].alt,pcom->sat[k].nref);
   */  

  shm_unlink(SHM_NAME);
  return 0;
}

// g++ -o geo_calc geo_calc.cc   -L. -lsgp4 -ljansson